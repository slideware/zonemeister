package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"zonemeister/internal/models"
	"zonemeister/internal/repository"
)

const (
	sessionCookieName = "session_id"
	sessionDuration   = 24 * time.Hour
)

// SessionManager handles session creation, validation, and destruction.
type SessionManager struct {
	repo   repository.SessionRepository
	secure bool // whether to set Secure flag on cookies
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(repo repository.SessionRepository, secureCookies bool) *SessionManager {
	return &SessionManager{
		repo:   repo,
		secure: secureCookies,
	}
}

// CreateSession generates a new session for the given user and sets the session cookie.
func (sm *SessionManager) CreateSession(ctx context.Context, w http.ResponseWriter, userID int64) (*models.Session, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &models.Session{
		ID:        token,
		UserID:    userID,
		ExpiresAt: now.Add(sessionDuration),
		CreatedAt: now,
	}

	if err := sm.repo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
	})

	return session, nil
}

// GetSession retrieves the session from the request cookie.
// Returns nil if no valid session exists.
func (sm *SessionManager) GetSession(ctx context.Context, r *http.Request) (*models.Session, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, nil
	}

	session, err := sm.repo.GetByID(ctx, cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session == nil {
		return nil, nil
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		sm.repo.Delete(ctx, session.ID)
		return nil, nil
	}

	return session, nil
}

// DestroySession deletes the session and clears the cookie.
func (sm *SessionManager) DestroySession(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}

	if err := sm.repo.Delete(ctx, cookie.Value); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	return nil
}

// DeleteUserSessions deletes all sessions belonging to the given user.
func (sm *SessionManager) DeleteUserSessions(ctx context.Context, userID int64) error {
	return sm.repo.DeleteByUserID(ctx, userID)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
