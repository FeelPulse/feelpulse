package ratelimit

import (
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	limiter := New(10) // 10 per minute
	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}
	if limiter.limit != 10 {
		t.Errorf("expected limit 10, got %d", limiter.limit)
	}
}

func TestLimiter_Disabled(t *testing.T) {
	limiter := New(0) // disabled

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !limiter.Allow("user1") {
			t.Error("disabled limiter should always allow")
		}
	}
}

func TestLimiter_UnderLimit(t *testing.T) {
	limiter := New(5) // 5 per minute

	// First 5 requests should pass
	for i := 0; i < 5; i++ {
		if !limiter.Allow("user1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestLimiter_OverLimit(t *testing.T) {
	limiter := New(3) // 3 per minute

	// First 3 should pass
	for i := 0; i < 3; i++ {
		limiter.Allow("user1")
	}

	// 4th should be blocked
	if limiter.Allow("user1") {
		t.Error("request over limit should be blocked")
	}
}

func TestLimiter_SeparateUsers(t *testing.T) {
	limiter := New(2) // 2 per minute

	// User1 uses their quota
	limiter.Allow("user1")
	limiter.Allow("user1")

	// User1 should be blocked
	if limiter.Allow("user1") {
		t.Error("user1 should be blocked")
	}

	// User2 should still be allowed
	if !limiter.Allow("user2") {
		t.Error("user2 should be allowed")
	}
}

func TestLimiter_WindowSlides(t *testing.T) {
	limiter := New(2)

	// Use quota
	limiter.Allow("user1")
	limiter.Allow("user1")

	// Should be blocked
	if limiter.Allow("user1") {
		t.Error("should be blocked initially")
	}

	// Manually expire old entries by manipulating internal state
	limiter.mu.Lock()
	if entries, ok := limiter.windows["user1"]; ok && len(entries) > 0 {
		// Set first entry to be > 1 minute old
		entries[0] = time.Now().Add(-2 * time.Minute)
	}
	limiter.mu.Unlock()

	// Now should allow (after window slides)
	if !limiter.Allow("user1") {
		t.Error("should be allowed after window slides")
	}
}

func TestLimiter_Cleanup(t *testing.T) {
	limiter := New(10)

	// Add some entries
	limiter.Allow("user1")
	limiter.Allow("user2")
	limiter.Allow("user3")

	// Manually expire all entries
	limiter.mu.Lock()
	for key := range limiter.windows {
		limiter.windows[key] = []time.Time{time.Now().Add(-2 * time.Minute)}
	}
	limiter.mu.Unlock()

	// Allow should clean up old entries
	limiter.Allow("user1")

	limiter.mu.Lock()
	user1Entries := len(limiter.windows["user1"])
	limiter.mu.Unlock()

	// Should have only 1 entry (the new one)
	if user1Entries != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", user1Entries)
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	limiter := New(100)
	done := make(chan bool)

	// Spawn multiple goroutines
	for i := 0; i < 10; i++ {
		go func(userID string) {
			for j := 0; j < 20; j++ {
				limiter.Allow(userID)
			}
			done <- true
		}("user" + string(rune('0'+i)))
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without deadlock/panic, test passes
}
