package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"zonemeister/internal/models"
)

// mockSessionRepo is a minimal in-memory session store for testing.
type mockSessionRepo struct {
	sessions map[string]*models.Session
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{sessions: make(map[string]*models.Session)}
}

func (m *mockSessionRepo) Create(_ context.Context, s *models.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionRepo) GetByID(_ context.Context, id string) (*models.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockSessionRepo) Delete(_ context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionRepo) DeleteByUserID(_ context.Context, userID int64) error {
	for id, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, id)
		}
	}
	return nil
}

func (m *mockSessionRepo) DeleteExpired(_ context.Context) error {
	now := time.Now().UTC()
	for id, s := range m.sessions {
		if now.After(s.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
	return nil
}

func TestSessionManagerCreateAndGet(t *testing.T) {
	repo := newMockSessionRepo()
	sm := NewSessionManager(repo, false)
	ctx := context.Background()

	// Create session.
	w := httptest.NewRecorder()
	session, err := sm.CreateSession(ctx, w, 42)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.UserID != 42 {
		t.Errorf("user_id = %d, want 42", session.UserID)
	}

	// Check cookie was set.
	resp := w.Result()
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}
	if !sessionCookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Error("cookie should be SameSite=Strict")
	}

	// Get session from request with the cookie.
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(sessionCookie)
	got, err := sm.GetSession(ctx, r)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.UserID != 42 {
		t.Errorf("got user_id = %d, want 42", got.UserID)
	}
}

func TestSessionManagerDestroy(t *testing.T) {
	repo := newMockSessionRepo()
	sm := NewSessionManager(repo, false)
	ctx := context.Background()

	w := httptest.NewRecorder()
	session, _ := sm.CreateSession(ctx, w, 1)

	// Build request with the session cookie.
	r := httptest.NewRequest("GET", "/", nil)
	resp := w.Result()
	for _, c := range resp.Cookies() {
		r.AddCookie(c)
	}

	// Destroy.
	w2 := httptest.NewRecorder()
	if err := sm.DestroySession(ctx, w2, r); err != nil {
		t.Fatalf("destroy session: %v", err)
	}

	// Session should be gone from repo.
	got, _ := repo.GetByID(ctx, session.ID)
	if got != nil {
		t.Error("session should have been deleted from repo")
	}
}

func TestSessionManagerGetExpired(t *testing.T) {
	repo := newMockSessionRepo()
	sm := NewSessionManager(repo, false)
	ctx := context.Background()

	// Manually insert an expired session.
	repo.sessions["expired-token"] = &models.Session{
		ID:        "expired-token",
		UserID:    1,
		ExpiresAt: time.Now().Add(-time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "expired-token"})

	got, err := sm.GetSession(ctx, r)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got != nil {
		t.Error("expired session should return nil")
	}
}
