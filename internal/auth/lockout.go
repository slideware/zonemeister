package auth

import (
	"sync"
	"time"
)

type loginAttempt struct {
	failures int
	lockedAt time.Time
}

// Lockout tracks failed login attempts and temporarily locks accounts.
type Lockout struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	maxFails int
	lockDur  time.Duration
}

// NewLockout creates a new Lockout tracker.
func NewLockout(maxFailures int, lockDuration time.Duration) *Lockout {
	return &Lockout{
		attempts: make(map[string]*loginAttempt),
		maxFails: maxFailures,
		lockDur:  lockDuration,
	}
}

// IsLocked returns true if the given email is currently locked out.
func (l *Lockout) IsLocked(email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[email]
	if !ok {
		return false
	}
	if a.failures < l.maxFails {
		return false
	}
	if time.Since(a.lockedAt) < l.lockDur {
		return true
	}
	// Lock has expired — clean up.
	delete(l.attempts, email)
	return false
}

// RecordFailure records a failed login attempt for the given email.
func (l *Lockout) RecordFailure(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[email]
	if !ok {
		a = &loginAttempt{}
		l.attempts[email] = a
	}
	a.failures++
	if a.failures >= l.maxFails {
		a.lockedAt = time.Now()
	}
}

// Reset clears the failed attempt counter for the given email.
func (l *Lockout) Reset(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, email)
}
