package middleware

import (
	"net/http"

	"github.com/justinas/nosurf"
)

// NewCSRF returns a nosurf CSRF protection middleware.
func NewCSRF(secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		csrfHandler := nosurf.New(next)
		csrfHandler.SetBaseCookie(http.Cookie{
			HttpOnly: true,
			Path:     "/",
			Secure:   secureCookies,
			SameSite: http.SameSiteStrictMode,
		})
		return csrfHandler
	}
}

// CSRFToken returns the CSRF token for the current request.
func CSRFToken(r *http.Request) string {
	return nosurf.Token(r)
}
