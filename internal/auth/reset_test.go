package auth

import (
	"testing"
	"time"
)

const (
	testSecret   = "test-secret-key"
	testPassword = "$2a$12$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012"
)

func TestGenerateAndVerifyResetToken(t *testing.T) {
	token := GenerateResetToken(42, testPassword, testSecret)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	err := VerifyResetToken(token, testSecret, 42, testPassword, time.Hour)
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
}

func TestValidateResetToken_ExtractsUserID(t *testing.T) {
	token := GenerateResetToken(99, testPassword, testSecret)

	userID, _, err := ValidateResetToken(token, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userID != 99 {
		t.Errorf("expected userID 99, got %d", userID)
	}
}

func TestVerifyResetToken_WrongSecret(t *testing.T) {
	token := GenerateResetToken(42, testPassword, testSecret)

	err := VerifyResetToken(token, "wrong-secret", 42, testPassword, time.Hour)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestVerifyResetToken_WrongUserID(t *testing.T) {
	token := GenerateResetToken(42, testPassword, testSecret)

	err := VerifyResetToken(token, testSecret, 99, testPassword, time.Hour)
	if err == nil {
		t.Error("expected error for wrong user ID")
	}
}

func TestVerifyResetToken_PasswordChanged(t *testing.T) {
	token := GenerateResetToken(42, testPassword, testSecret)

	newPassword := "$2a$12$ZZZZZZZZZZZZZZZZZZZZZZabcdefghijklmnopqrstuvwxyz012345"
	err := VerifyResetToken(token, testSecret, 42, newPassword, time.Hour)
	if err == nil {
		t.Error("expected error when password has changed")
	}
}

func TestVerifyResetToken_Expired(t *testing.T) {
	token := GenerateResetToken(42, testPassword, testSecret)

	// Use a zero max age to simulate expiration.
	err := VerifyResetToken(token, testSecret, 42, testPassword, 0)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateResetToken_InvalidFormat(t *testing.T) {
	_, _, err := ValidateResetToken("not-a-valid-token", testSecret, time.Hour)
	if err == nil {
		t.Error("expected error for invalid token format")
	}
}

func TestVerifyResetToken_InvalidFormat(t *testing.T) {
	err := VerifyResetToken("garbage", testSecret, 42, testPassword, time.Hour)
	if err == nil {
		t.Error("expected error for invalid token")
	}
}
