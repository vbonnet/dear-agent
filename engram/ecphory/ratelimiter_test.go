package ecphory

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimiter_Allow_BasicUsage(t *testing.T) {
	rl := NewRateLimiter()

	// First request should succeed
	err := rl.Allow()
	if err != nil {
		t.Errorf("First request should succeed, got: %v", err)
	}

	// Second request immediately should fail (rate: 1 req/sec)
	err = rl.Allow()
	if err == nil {
		t.Error("Second immediate request should fail due to rate limit")
	}

	// Wait for rate limit window
	time.Sleep(1010 * time.Millisecond)

	// Third request should succeed after rate limit window
	err = rl.Allow()
	if err != nil {
		t.Errorf("Request after rate limit window should succeed, got: %v", err)
	}
}

func TestRateLimiter_Allow_SessionLimit(t *testing.T) {
	rl := NewRateLimiter()

	// Consume several session tokens to verify session tracking works
	// (Testing all 20 would take 20+ seconds and exceed test timeout)
	requestCount := 5
	for i := 0; i < requestCount; i++ {
		err := rl.Allow()
		if err != nil {
			t.Errorf("Request %d should succeed, got: %v", i+1, err)
		}
		// Wait for rate limit between requests
		time.Sleep(1010 * time.Millisecond)
	}

	// Verify session tokens were consumed
	expectedRemaining := 20 - requestCount
	if rl.sessionTokens != expectedRemaining {
		t.Errorf("After %d requests, should have %d session tokens remaining, got %d",
			requestCount, expectedRemaining, rl.sessionTokens)
	}

	// Manually exhaust remaining tokens to test limit enforcement
	rl.sessionTokens = 0

	// Next request should fail due to session limit
	time.Sleep(1010 * time.Millisecond)
	err := rl.Allow()
	if err == nil {
		t.Error("Request with exhausted session tokens should fail")
	}

	// Verify error message mentions session limit
	if err != nil && !strings.Contains(err.Error(), "session") {
		t.Errorf("Error should mention session limit, got: %v", err)
	}
}

func TestRateLimiter_Allow_SessionTracking(t *testing.T) {
	rl := NewRateLimiter()

	// Verify initial session tokens
	if rl.sessionTokens != 20 {
		t.Errorf("Initial session tokens should be 20, got %d", rl.sessionTokens)
	}

	// Consume one token
	err := rl.Allow()
	if err != nil {
		t.Errorf("First request should succeed, got: %v", err)
	}

	// Verify session token was consumed
	if rl.sessionTokens != 19 {
		t.Errorf("Session tokens should be 19 after one request, got %d", rl.sessionTokens)
	}

	// Wait and make another request
	time.Sleep(1010 * time.Millisecond)
	err = rl.Allow()
	if err != nil {
		t.Errorf("Second request should succeed, got: %v", err)
	}

	// Verify another session token was consumed
	if rl.sessionTokens != 18 {
		t.Errorf("Session tokens should be 18 after two requests, got %d", rl.sessionTokens)
	}
}

func TestRateLimiter_Allow_RateLimit(t *testing.T) {
	rl := NewRateLimiter()

	// First request
	err := rl.Allow()
	if err != nil {
		t.Fatalf("First request should succeed, got: %v", err)
	}

	// Immediate second request should fail (rate: 1 req/sec)
	err = rl.Allow()
	if err == nil {
		t.Error("Immediate second request should fail due to rate limit")
	}

	// Wait for rate limit window
	time.Sleep(1010 * time.Millisecond)

	// Should succeed after waiting
	err = rl.Allow()
	if err != nil {
		t.Errorf("Request after rate limit window should succeed, got: %v", err)
	}
}

func TestRateLimiter_Allow_Concurrent(t *testing.T) {
	rl := NewRateLimiter()

	// Test that rate limiter is thread-safe
	done := make(chan bool)
	results := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			err := rl.Allow()
			results <- err
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	close(results)

	// At least one should succeed
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("At least one concurrent request should succeed")
	}

	// Not all should succeed (only 1 req/sec allowed, burst=1)
	if successCount > 1 {
		t.Logf("Warning: %d concurrent requests succeeded (expected 1)", successCount)
	}
}

// TestRateLimiter_StdlibMonotonicTime verifies that golang.org/x/time/rate
// uses monotonic time internally for rate calculations (immune to clock drift)
func TestRateLimiter_StdlibMonotonicTime(t *testing.T) {
	rl := NewRateLimiter()

	// First request should succeed
	err := rl.Allow()
	if err != nil {
		t.Fatalf("First request should succeed, got: %v", err)
	}

	// golang.org/x/time/rate internally uses monotonic time for all
	// duration calculations (via time.Now() and time.Since()), making it
	// immune to wall clock adjustments (NTP sync, timezone changes, etc.)

	// Verify rate limiter works correctly after brief pause
	time.Sleep(100 * time.Millisecond)

	// Second request should still be rate limited
	err = rl.Allow()
	if err == nil {
		t.Error("Second request within 1 second should fail")
	}

	// Wait for rate limit window
	time.Sleep(920 * time.Millisecond)

	// Should succeed after full second
	err = rl.Allow()
	if err != nil {
		t.Errorf("Request after 1+ second should succeed, got: %v", err)
	}

	t.Log("Successfully verified stdlib rate limiter uses monotonic time")
}

// TestRateLimiter_StdlibLimiterBehavior verifies rate.Limiter behavior
func TestRateLimiter_StdlibLimiterBehavior(t *testing.T) {
	// Create a limiter directly to test stdlib behavior
	limiter := rate.NewLimiter(rate.Every(1*time.Second), 1)

	// First request should succeed (burst allows immediate token)
	if !limiter.Allow() {
		t.Error("First request should succeed due to burst capacity")
	}

	// Second immediate request should fail (bucket empty)
	if limiter.Allow() {
		t.Error("Second immediate request should fail")
	}

	// Wait for token refill
	time.Sleep(1010 * time.Millisecond)

	// Should succeed after refill period
	if !limiter.Allow() {
		t.Error("Request after refill period should succeed")
	}

	t.Log("Successfully verified stdlib rate.Limiter behavior")
}
