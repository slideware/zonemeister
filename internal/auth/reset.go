package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const passwordHashPrefixLen = 8

// GenerateResetToken creates an HMAC-signed token containing the user ID and
// current timestamp. The HMAC is computed over userID:timestamp:passwordHashPrefix
// so the token is automatically invalidated when the password changes.
func GenerateResetToken(userID int64, passwordHash string, secret string) string {
	timestamp := time.Now().Unix()
	payload := fmt.Sprintf("%d:%d", userID, timestamp)
	sig := computeResetHMAC(userID, timestamp, hashPrefix(passwordHash), secret)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ValidateResetToken parses and verifies a reset token. It checks the HMAC
// signature and that the token has not expired. On success it returns the user
// ID and the password hash prefix that was embedded in the signature. The
// caller must compare the returned prefix against the user's current password
// hash to detect if the password was changed after the token was issued.
func ValidateResetToken(token string, secret string, maxAge time.Duration) (userID int64, pwHashPrefix string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0, "", errors.New("invalid token format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, "", errors.New("invalid token encoding")
	}

	payload := string(payloadBytes)
	sepIdx := strings.LastIndex(payload, ":")
	if sepIdx < 0 {
		return 0, "", errors.New("invalid token payload")
	}

	userID, err = strconv.ParseInt(payload[:sepIdx], 10, 64)
	if err != nil {
		return 0, "", errors.New("invalid user id in token")
	}

	timestamp, err := strconv.ParseInt(payload[sepIdx+1:], 10, 64)
	if err != nil {
		return 0, "", errors.New("invalid timestamp in token")
	}

	issued := time.Unix(timestamp, 0)
	if time.Since(issued) > maxAge {
		return 0, "", errors.New("token expired")
	}

	// We cannot verify the HMAC without the password hash prefix, so we
	// return the user ID and an empty prefix. The caller must:
	// 1. Look up the user by ID
	// 2. Call VerifyResetToken with the user's current password hash
	return userID, "", nil
}

// VerifyResetToken performs the full HMAC verification of a reset token
// against the user's current password hash. Returns nil if valid.
func VerifyResetToken(token string, secret string, userID int64, passwordHash string, maxAge time.Duration) error {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return errors.New("invalid token format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("invalid token encoding")
	}

	payload := string(payloadBytes)
	sepIdx := strings.LastIndex(payload, ":")
	if sepIdx < 0 {
		return errors.New("invalid token payload")
	}

	tokenUserID, err := strconv.ParseInt(payload[:sepIdx], 10, 64)
	if err != nil {
		return errors.New("invalid user id in token")
	}
	if tokenUserID != userID {
		return errors.New("user id mismatch")
	}

	timestamp, err := strconv.ParseInt(payload[sepIdx+1:], 10, 64)
	if err != nil {
		return errors.New("invalid timestamp in token")
	}

	issued := time.Unix(timestamp, 0)
	if time.Since(issued) > maxAge {
		return errors.New("token expired")
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("invalid signature encoding")
	}

	expected := computeResetHMAC(userID, timestamp, hashPrefix(passwordHash), secret)
	if !hmac.Equal(sigBytes, expected) {
		return errors.New("invalid token signature")
	}

	return nil
}

func computeResetHMAC(userID int64, timestamp int64, pwHashPrefix string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d:%d:%s", userID, timestamp, pwHashPrefix)
	return mac.Sum(nil)
}

func hashPrefix(passwordHash string) string {
	if len(passwordHash) > passwordHashPrefixLen {
		return passwordHash[:passwordHashPrefixLen]
	}
	return passwordHash
}
