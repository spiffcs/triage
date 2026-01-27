package github

import (
	"errors"
	"sync"
	"time"
)

// ErrRateLimited is returned when the GitHub API rate limit has been exceeded.
var ErrRateLimited = errors.New("rate limited")

// RateLimitState tracks the global rate limit state for GitHub API requests.
type RateLimitState struct {
	mu        sync.RWMutex
	limited   bool
	resetAt   time.Time
	remaining int
	limit     int
}

var globalRateLimitState = &RateLimitState{}

// IsLimited returns true if we are currently rate limited.
func (s *RateLimitState) IsLimited() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.limited {
		return false
	}

	// Check if rate limit has reset
	if time.Now().After(s.resetAt) {
		return false
	}

	return true
}

// SetLimited sets the rate limit state.
func (s *RateLimitState) SetLimited(limited bool, resetAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limited = limited
	s.resetAt = resetAt
}

// Update updates the rate limit state from response headers.
func (s *RateLimitState) Update(remaining, limit int, resetAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remaining = remaining
	s.limit = limit
	s.resetAt = resetAt

	// If remaining is 0, mark as limited
	if remaining == 0 {
		s.limited = true
	}
}

// GetStatus returns the current rate limit status.
func (s *RateLimitState) GetStatus() (remaining, limit int, resetAt time.Time, limited bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remaining, s.limit, s.resetAt, s.limited && time.Now().Before(s.resetAt)
}

// GetRateLimitStatus returns the global rate limit status.
func GetRateLimitStatus() (remaining, limit int, resetAt time.Time, limited bool) {
	return globalRateLimitState.GetStatus()
}
