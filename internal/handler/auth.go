package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"zonemeister/internal/auth"
	"zonemeister/internal/mail"
	"zonemeister/internal/middleware"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"

	"github.com/pquerna/otp/totp"
)

const resetTokenMaxAge = 1 * time.Hour

type resetPasswordData struct {
	Token string
	Error string
}

const totpPendingCookie = "totp_pending"

// AuthHandler handles login, logout, and password reset.
type AuthHandler struct {
	userRepo      repository.UserRepository
	sessions      *auth.SessionManager
	renderer      *templates.Renderer
	lockout       *auth.Lockout
	secure        bool
	mailer        *mail.Mailer
	baseURL       string
	sessionSecret string
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userRepo repository.UserRepository, sessions *auth.SessionManager, renderer *templates.Renderer, lockout *auth.Lockout, secureCookies bool, mailer *mail.Mailer, baseURL string, sessionSecret string) *AuthHandler {
	return &AuthHandler{
		userRepo:      userRepo,
		sessions:      sessions,
		renderer:      renderer,
		lockout:       lockout,
		secure:        secureCookies,
		mailer:        mailer,
		baseURL:       baseURL,
		sessionSecret: sessionSecret,
	}
}

// LoginForm renders the login page.
func (h *AuthHandler) LoginForm(w http.ResponseWriter, r *http.Request) {
	if middleware.UserFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if err := h.renderer.Render(w, r, w, "login", nil); err != nil {
		slog.Error("render login form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Login handles login form submission.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	if h.lockout.IsLocked(email) {
		w.WriteHeader(http.StatusTooManyRequests)
		if err := h.renderer.Render(w, r, w, "login", "Account temporarily locked. Try again later."); err != nil {
			slog.Error("render login lockout", "error", err)
		}
		return
	}

	user, err := h.userRepo.GetByEmail(r.Context(), email)
	if err != nil {
		slog.Error("login: get user", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if user == nil || auth.CheckPassword(password, user.PasswordHash) != nil {
		h.lockout.RecordFailure(email)
		w.WriteHeader(http.StatusUnauthorized)
		if err := h.renderer.Render(w, r, w, "login", "Invalid email or password."); err != nil {
			slog.Error("render login error", "error", err)
		}
		return
	}

	h.lockout.Reset(email)

	// If 2FA is enabled, show TOTP verification instead of completing login.
	if user.TOTPEnabled {
		h.setTOTPPendingCookie(w, user.ID)
		if err := h.renderer.Render(w, r, w, "totp-verify", nil); err != nil {
			slog.Error("render TOTP verify", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	h.completeLogin(w, r, user.ID)
}

// VerifyTOTP handles the TOTP code verification step during login.
func (h *AuthHandler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getTOTPPendingUserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		slog.Error("totp verify: get user", "error", err)
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	code := r.FormValue("code")
	if !totp.Validate(code, user.TOTPSecret) {
		h.lockout.RecordFailure(user.Email)
		if h.lockout.IsLocked(user.Email) {
			h.clearTOTPPendingCookie(w)
			w.WriteHeader(http.StatusTooManyRequests)
			if err := h.renderer.Render(w, r, w, "login", "Account temporarily locked. Try again later."); err != nil {
				slog.Error("render login lockout", "error", err)
			}
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		if err := h.renderer.Render(w, r, w, "totp-verify", "Invalid code. Please try again."); err != nil {
			slog.Error("render TOTP verify error", "error", err)
		}
		return
	}

	h.clearTOTPPendingCookie(w)
	h.completeLogin(w, r, user.ID)
}

// completeLogin finalizes the login by creating a session and redirecting.
func (h *AuthHandler) completeLogin(w http.ResponseWriter, r *http.Request, userID int64) {
	// Destroy any existing session to prevent session fixation.
	h.sessions.DestroySession(r.Context(), w, r)

	if _, err := h.sessions.CreateSession(r.Context(), w, userID); err != nil {
		slog.Error("login: create session", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Logged in successfully.")
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout destroys the session and redirects to login.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sessions.DestroySession(r.Context(), w, r); err != nil {
		slog.Error("logout: destroy session", "error", err)
	}

	middleware.SetFlash(w, "Logged out.")
	http.Redirect(w, r, "/login", http.StatusFound)
}

// setTOTPPendingCookie sets a short-lived HMAC-signed cookie with the user ID.
func (h *AuthHandler) setTOTPPendingCookie(w http.ResponseWriter, userID int64) {
	expiry := time.Now().Add(5 * time.Minute).Unix()
	value := fmt.Sprintf("%d:%d", userID, expiry)
	http.SetCookie(w, &http.Cookie{
		Name:     totpPendingCookie,
		Value:    middleware.SignValue(value),
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})
}

// getTOTPPendingUserID reads and validates the TOTP pending cookie.
func (h *AuthHandler) getTOTPPendingUserID(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(totpPendingCookie)
	if err != nil {
		return 0, false
	}
	value, ok := middleware.VerifyValue(cookie.Value)
	if !ok {
		return 0, false
	}
	var userID, expiry int64
	if _, err := fmt.Sscanf(value, "%d:%d", &userID, &expiry); err != nil {
		return 0, false
	}
	if time.Now().Unix() > expiry {
		return 0, false
	}
	return userID, true
}

// clearTOTPPendingCookie removes the TOTP pending cookie.
func (h *AuthHandler) clearTOTPPendingCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     totpPendingCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// ForgotPasswordForm renders the forgot password page.
func (h *AuthHandler) ForgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if err := h.renderer.Render(w, r, w, "forgot-password", nil); err != nil {
		slog.Error("render forgot password form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ForgotPassword handles the forgot password form submission.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	email := r.FormValue("email")

	// Always show the same confirmation to prevent email enumeration.
	showConfirmation := func() {
		if err := h.renderer.Render(w, r, w, "forgot-password-sent", nil); err != nil {
			slog.Error("render forgot password sent", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}

	user, err := h.userRepo.GetByEmail(r.Context(), email)
	if err != nil {
		slog.Error("forgot password: get user", "error", err)
		showConfirmation()
		return
	}
	if user == nil {
		showConfirmation()
		return
	}

	token := auth.GenerateResetToken(user.ID, user.PasswordHash, h.sessionSecret)
	resetURL := h.baseURL + "/reset-password?token=" + token

	body := fmt.Sprintf("You requested a password reset.\n\nClick the link below to set a new password:\n%s\n\nThis link expires in 1 hour.\n\nIf you did not request this, you can safely ignore this email.", resetURL)

	if err := h.mailer.Send(context.Background(), user.Email, "Password Reset", body); err != nil {
		slog.Error("forgot password: send email", "error", err, "email", user.Email)
	}

	showConfirmation()
}

// ResetPasswordForm renders the reset password page.
func (h *AuthHandler) ResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Quick validation: check format and expiry (full HMAC check happens on submit).
	_, _, err := auth.ValidateResetToken(token, h.sessionSecret, resetTokenMaxAge)
	if err != nil {
		slog.Warn("reset password: invalid token", "error", err)
		middleware.SetFlash(w, "Invalid or expired reset link.")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := h.renderer.Render(w, r, w, "reset-password", resetPasswordData{Token: token}); err != nil {
		slog.Error("render reset password form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ResetPassword handles the reset password form submission.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	renderError := func(msg string) {
		w.WriteHeader(http.StatusBadRequest)
		if err := h.renderer.Render(w, r, w, "reset-password", resetPasswordData{Token: token, Error: msg}); err != nil {
			slog.Error("render reset password error", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}

	if password != confirmPassword {
		renderError("Passwords do not match.")
		return
	}

	if err := auth.ValidatePassword(password); err != nil {
		renderError(err.Error())
		return
	}

	userID, _, err := auth.ValidateResetToken(token, h.sessionSecret, resetTokenMaxAge)
	if err != nil {
		slog.Warn("reset password: invalid token", "error", err)
		middleware.SetFlash(w, "Invalid or expired reset link.")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		slog.Error("reset password: get user", "error", err)
		middleware.SetFlash(w, "Invalid or expired reset link.")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Full HMAC verification with the user's current password hash.
	if err := auth.VerifyResetToken(token, h.sessionSecret, user.ID, user.PasswordHash, resetTokenMaxAge); err != nil {
		slog.Warn("reset password: token verification failed", "error", err)
		middleware.SetFlash(w, "Invalid or expired reset link.")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("reset password: hash password", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user.PasswordHash = hash
	if err := h.userRepo.Update(r.Context(), user); err != nil {
		slog.Error("reset password: update user", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Invalidate all existing sessions for this user.
	if err := h.sessions.DeleteUserSessions(r.Context(), user.ID); err != nil {
		slog.Error("reset password: delete sessions", "error", err)
	}

	middleware.SetFlash(w, "Password has been reset. Please log in.")
	http.Redirect(w, r, "/login", http.StatusFound)
}
