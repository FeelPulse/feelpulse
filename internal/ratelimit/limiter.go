package ratelimit

import (
	"sync"
	"time"
)

const (
	// WindowDuration is the sliding window size
	WindowDuration = time.Minute
)

// Limiter implements a per-user sliding window rate limiter
type Limiter struct {
	limit   int                      // max requests per window (0 = disabled)
	windows map[string][]time.Time   // userID -> timestamps of requests
	mu      sync.Mutex
}

// New creates a new rate limiter
// limit is the maximum number of requests per minute per user
// limit <= 0 means rate limiting is disabled
func New(limit int) *Limiter {
	return &Limiter{
		limit:   limit,
		windows: make(map[string][]time.Time),
	}
}

// Allow checks if a request from the given user should be allowed
// Returns true if allowed, false if rate limited
func (l *Limiter) Allow(userID string) bool {
	// Disabled
	if l.limit <= 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-WindowDuration)

	// Get existing timestamps for this user
	timestamps, exists := l.windows[userID]

	// Clean up old timestamps (outside the window)
	if exists {
		valid := timestamps[:0]
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}
		timestamps = valid
		// Clean up empty entries to prevent memory leak
		if len(timestamps) == 0 {
			delete(l.windows, userID)
			// Re-add since we know we're under limit
			l.windows[userID] = []time.Time{now}
			return true
		}
	}

	// Check if under limit
	if len(timestamps) >= l.limit {
		l.windows[userID] = timestamps
		return false
	}

	// Add current request
	timestamps = append(timestamps, now)
	l.windows[userID] = timestamps

	return true
}

// Remaining returns how many requests the user has left in the current window
func (l *Limiter) Remaining(userID string) int {
	if l.limit <= 0 {
		return -1 // unlimited
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-WindowDuration)

	timestamps, exists := l.windows[userID]
	if !exists {
		return l.limit
	}

	// Count valid timestamps
	count := 0
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			count++
		}
	}

	remaining := l.limit - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// Reset clears all rate limit data for a user
func (l *Limiter) Reset(userID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, userID)
}

// ResetAll clears all rate limit data
func (l *Limiter) ResetAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.windows = make(map[string][]time.Time)
}
