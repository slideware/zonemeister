package auth_test

import (
	"testing"
	"time"

	"zonemeister/internal/auth"
)

func TestLockout_NotLockedInitially(t *testing.T) {
	l := auth.NewLockout(3, time.Minute)
	if l.IsLocked("user@example.com") {
		t.Error("expected not locked initially")
	}
}

func TestLockout_LocksAfterMaxFailures(t *testing.T) {
	l := auth.NewLockout(3, time.Minute)
	email := "user@example.com"

	l.RecordFailure(email)
	l.RecordFailure(email)
	if l.IsLocked(email) {
		t.Error("should not be locked after 2 failures")
	}

	l.RecordFailure(email)
	if !l.IsLocked(email) {
		t.Error("should be locked after 3 failures")
	}
}

func TestLockout_ResetClearsLock(t *testing.T) {
	l := auth.NewLockout(2, time.Minute)
	email := "user@example.com"

	l.RecordFailure(email)
	l.RecordFailure(email)
	if !l.IsLocked(email) {
		t.Fatal("should be locked")
	}

	l.Reset(email)
	if l.IsLocked(email) {
		t.Error("should not be locked after reset")
	}
}

func TestLockout_ExpiresAfterDuration(t *testing.T) {
	l := auth.NewLockout(1, 10*time.Millisecond)
	email := "user@example.com"

	l.RecordFailure(email)
	if !l.IsLocked(email) {
		t.Fatal("should be locked")
	}

	time.Sleep(20 * time.Millisecond)
	if l.IsLocked(email) {
		t.Error("should not be locked after expiry")
	}
}

func TestLockout_IndependentEmails(t *testing.T) {
	l := auth.NewLockout(2, time.Minute)

	l.RecordFailure("a@example.com")
	l.RecordFailure("a@example.com")
	l.RecordFailure("b@example.com")

	if !l.IsLocked("a@example.com") {
		t.Error("a should be locked")
	}
	if l.IsLocked("b@example.com") {
		t.Error("b should not be locked")
	}
}
