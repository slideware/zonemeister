package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
)

const flashCookieName = "flash"

var flashSecret []byte

// SetFlashSecret sets the HMAC key used to sign flash cookies.
// Must be called before any SetFlash/GetFlash calls.
func SetFlashSecret(secret string) {
	flashSecret = []byte(secret)
}

// SignValue creates an HMAC-signed, base64-encoded value.
func SignValue(value string) string {
	return signValue(value)
}

// VerifyValue checks an HMAC-signed value and returns the original.
func VerifyValue(signed string) (string, bool) {
	return verifyValue(signed)
}

func signValue(value string) string {
	mac := hmac.New(sha256.New, flashSecret)
	mac.Write([]byte(value))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	val := base64.RawURLEncoding.EncodeToString([]byte(value))
	return val + "." + sig
}

func verifyValue(signed string) (string, bool) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	value, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, flashSecret)
	mac.Write(value)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return "", false
	}
	return string(value), true
}

// SetFlash sets an HMAC-signed flash message cookie.
func SetFlash(w http.ResponseWriter, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    signValue(message),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   60,
	})
}

// GetFlash reads and clears the flash message from the request.
// Returns empty string if the cookie is missing or the signature is invalid.
func GetFlash(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil {
		return ""
	}

	// Clear the cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	value, ok := verifyValue(cookie.Value)
	if !ok {
		return ""
	}
	return value
}
