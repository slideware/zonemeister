package auth

import (
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	password := "mysecretpassword"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == password {
		t.Fatal("hash should differ from plaintext")
	}

	// Correct password should match.
	if err := CheckPassword(password, hash); err != nil {
		t.Errorf("correct password should match: %v", err)
	}

	// Wrong password should not match.
	if err := CheckPassword("wrongpassword", hash); err == nil {
		t.Error("wrong password should not match")
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	hash1, _ := HashPassword("same")
	hash2, _ := HashPassword("same")
	if hash1 == hash2 {
		t.Error("two hashes of the same password should differ (different salts)")
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid", "Abcdefghi1", false},
		{"too short", "Abcdefg1", true},
		{"no uppercase", "abcdefghij1", true},
		{"no lowercase", "ABCDEFGHIJ1", true},
		{"no digit", "Abcdefghijk", true},
		{"exactly 10 chars valid", "Abcdefghi0", false},
		{"long valid", "MyStr0ngPassword!!", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}
