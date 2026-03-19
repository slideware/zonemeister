package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"zonemeister/internal/middleware"
)

func TestSecurityHeaders(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := map[string]string{
		"X-Frame-Options":                "DENY",
		"X-Content-Type-Options":         "nosniff",
		"Referrer-Policy":                "strict-origin-when-cross-origin",
		"X-Permitted-Cross-Domain-Policies": "none",
		"Content-Security-Policy":        "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:",
	}

	for header, want := range expected {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}
