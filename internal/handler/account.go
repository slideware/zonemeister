package handler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"log/slog"
	"net/http"

	"zonemeister/internal/auth"
	"zonemeister/internal/middleware"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// AccountHandler handles account settings (change password, 2FA).
type AccountHandler struct {
	userRepo repository.UserRepository
	renderer *templates.Renderer
}

// NewAccountHandler creates a new AccountHandler.
func NewAccountHandler(userRepo repository.UserRepository, renderer *templates.Renderer) *AccountHandler {
	return &AccountHandler{
		userRepo: userRepo,
		renderer: renderer,
	}
}

// AccountData holds template data for the account page.
type AccountData struct {
	Error       string
	TOTPEnabled bool
}

// TOTPSetupData holds template data for the TOTP setup page.
type TOTPSetupData struct {
	QRCode string // base64-encoded PNG
	Secret string // text secret for manual entry
	Error  string
}

// Show renders the account settings page.
func (h *AccountHandler) Show(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	data := AccountData{TOTPEnabled: user != nil && user.TOTPEnabled}
	if err := h.renderer.Render(w, r, w, "account", data); err != nil {
		slog.Error("render account", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ChangePassword handles password change form submission.
func (h *AccountHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	renderError := func(msg string) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		data := AccountData{Error: msg, TOTPEnabled: user.TOTPEnabled}
		if err := h.renderer.Render(w, r, w, "account", data); err != nil {
			slog.Error("render account error", "error", err)
		}
	}

	if newPassword != confirmPassword {
		renderError("New passwords do not match.")
		return
	}

	if err := auth.CheckPassword(currentPassword, user.PasswordHash); err != nil {
		renderError("Current password is incorrect.")
		return
	}

	if err := auth.ValidatePassword(newPassword); err != nil {
		renderError(err.Error())
		return
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		slog.Error("hash password", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user.PasswordHash = hash
	if err := h.userRepo.Update(r.Context(), user); err != nil {
		slog.Error("update user password", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Password changed successfully.")
	http.Redirect(w, r, "/account", http.StatusFound)
}

// SetupTOTP generates a TOTP secret and renders the setup page with QR code.
func (h *AccountHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Netnod DNS Portal",
		AccountName: user.Email,
	})
	if err != nil {
		slog.Error("generate TOTP key", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Save secret (not yet enabled).
	user.TOTPSecret = key.Secret()
	if err := h.userRepo.Update(r.Context(), user); err != nil {
		slog.Error("save TOTP secret", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Generate QR code as base64 PNG.
	img, err := key.Image(200, 200)
	if err != nil {
		slog.Error("generate TOTP QR image", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		slog.Error("encode TOTP QR PNG", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := TOTPSetupData{
		QRCode: base64.StdEncoding.EncodeToString(buf.Bytes()),
		Secret: key.Secret(),
	}
	if err := h.renderer.Render(w, r, w, "totp-setup", data); err != nil {
		slog.Error("render TOTP setup", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// EnableTOTP verifies a TOTP code and enables 2FA for the user.
func (h *AccountHandler) EnableTOTP(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	code := r.FormValue("code")
	if !totp.Validate(code, user.TOTPSecret) {
		// Re-render setup page with error.
		qrData, err := h.generateQRData(user.TOTPSecret, user.Email)
		if err != nil {
			slog.Error("regenerate QR for error", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		qrData.Error = "Invalid code. Please try again."
		w.WriteHeader(http.StatusUnprocessableEntity)
		if err := h.renderer.Render(w, r, w, "totp-setup", qrData); err != nil {
			slog.Error("render TOTP setup error", "error", err)
		}
		return
	}

	user.TOTPEnabled = true
	if err := h.userRepo.Update(r.Context(), user); err != nil {
		slog.Error("enable TOTP", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Two-factor authentication enabled.")
	http.Redirect(w, r, "/account", http.StatusFound)
}

// DisableTOTP disables 2FA after verifying password and TOTP code.
func (h *AccountHandler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	password := r.FormValue("password")
	code := r.FormValue("code")

	renderError := func(msg string) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		data := AccountData{Error: msg, TOTPEnabled: user.TOTPEnabled}
		if err := h.renderer.Render(w, r, w, "account", data); err != nil {
			slog.Error("render account error", "error", err)
		}
	}

	if err := auth.CheckPassword(password, user.PasswordHash); err != nil {
		renderError("Password is incorrect.")
		return
	}

	if !totp.Validate(code, user.TOTPSecret) {
		renderError("Invalid TOTP code.")
		return
	}

	user.TOTPEnabled = false
	user.TOTPSecret = ""
	if err := h.userRepo.Update(r.Context(), user); err != nil {
		slog.Error("disable TOTP", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Two-factor authentication disabled.")
	http.Redirect(w, r, "/account", http.StatusFound)
}

// generateQRData creates a TOTPSetupData from an existing secret (for re-rendering on error).
func (h *AccountHandler) generateQRData(secret, email string) (*TOTPSetupData, error) {
	// Reconstruct the otpauth URL so we can generate the QR image.
	u := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		"Netnod+DNS+Portal", email, secret, "Netnod+DNS+Portal")
	key, err := otp.NewKeyFromURL(u)
	if err != nil {
		return nil, err
	}
	img, err := key.Image(200, 200)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return &TOTPSetupData{
		QRCode: base64.StdEncoding.EncodeToString(buf.Bytes()),
		Secret: secret,
	}, nil
}
