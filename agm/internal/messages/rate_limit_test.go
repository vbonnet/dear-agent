package messages

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter(t *testing.T) {
	t.Run("creates with correct defaults", func(t *testing.T) {
		rl := NewRateLimiter("test-sender", 10, 15)
		require.NotNil(t, rl)
		assert.Equal(t, "test-sender", rl.senderName)
		assert.Equal(t, 10, rl.messagesPerMin)
		assert.Equal(t, 15, rl.burst)
		assert.Equal(t, 15, rl.tokens) // starts full
	})

	t.Run("starts with full token bucket", func(t *testing.T) {
		rl := NewRateLimiter("sender", 5, 3)
		assert.Equal(t, 3, rl.tokens)
	})
}

func TestRateLimiterAllow(t *testing.T) {
	t.Run("allows messages up to burst limit", func(t *testing.T) {
		rl := NewRateLimiter("sender", 10, 3)

		// First 3 should be allowed (burst = 3)
		for i := 0; i < 3; i++ {
			allowed, remaining, err := rl.Allow()
			assert.True(t, allowed, "message %d should be allowed", i)
			assert.NoError(t, err)
			assert.Equal(t, 2-i, remaining)
		}
	})

	t.Run("rejects after burst exhausted", func(t *testing.T) {
		rl := NewRateLimiter("sender", 10, 2)

		// Exhaust burst
		rl.Allow()
		rl.Allow()

		// Should be rejected
		allowed, remaining, err := rl.Allow()
		assert.False(t, allowed)
		assert.Equal(t, 0, remaining)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rate limit exceeded")
		assert.Contains(t, err.Error(), "sender")
	})

	t.Run("error message includes sender name", func(t *testing.T) {
		rl := NewRateLimiter("my-session", 5, 1)
		rl.Allow() // exhaust

		_, _, err := rl.Allow()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "my-session")
		assert.Contains(t, err.Error(), "5 messages per minute")
	})

	t.Run("returns remaining token count", func(t *testing.T) {
		rl := NewRateLimiter("sender", 10, 5)

		_, remaining, _ := rl.Allow()
		assert.Equal(t, 4, remaining)
		_, remaining, _ = rl.Allow()
		assert.Equal(t, 3, remaining)
		_, remaining, _ = rl.Allow()
		assert.Equal(t, 2, remaining)
	})

	t.Run("burst of one allows exactly one", func(t *testing.T) {
		rl := NewRateLimiter("sender", 10, 1)

		allowed, _, err := rl.Allow()
		assert.True(t, allowed)
		assert.NoError(t, err)

		allowed, _, err = rl.Allow()
		assert.False(t, allowed)
		assert.Error(t, err)
	})
}

func TestRateLimiterConcurrency(t *testing.T) {
	rl := NewRateLimiter("concurrent-sender", 100, 50)

	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _ := rl.Allow()
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// All 50 should be allowed (burst = 50)
	assert.Equal(t, 50, allowedCount)
}

func TestGetRateLimiter(t *testing.T) {
	// Reset global state for test isolation
	rateLimitersMu.Lock()
	originalLimiters := rateLimiters
	rateLimiters = make(map[string]*RateLimiter)
	rateLimitersMu.Unlock()

	t.Cleanup(func() {
		rateLimitersMu.Lock()
		rateLimiters = originalLimiters
		rateLimitersMu.Unlock()
	})

	t.Run("creates new limiter for unknown sender", func(t *testing.T) {
		rl := GetRateLimiter("new-sender")
		require.NotNil(t, rl)
		assert.Equal(t, "new-sender", rl.senderName)
		assert.Equal(t, 10, rl.messagesPerMin)
		assert.Equal(t, 15, rl.burst)
	})

	t.Run("returns same limiter for same sender", func(t *testing.T) {
		rl1 := GetRateLimiter("same-sender")
		rl2 := GetRateLimiter("same-sender")
		assert.Same(t, rl1, rl2)
	})

	t.Run("returns different limiters for different senders", func(t *testing.T) {
		rl1 := GetRateLimiter("sender-a")
		rl2 := GetRateLimiter("sender-b")
		assert.NotSame(t, rl1, rl2)
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		var wg sync.WaitGroup
		limiters := make([]*RateLimiter, 20)

		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				limiters[idx] = GetRateLimiter("shared-sender")
			}(i)
		}

		wg.Wait()

		// All should return the same instance
		for i := 1; i < 20; i++ {
			assert.Same(t, limiters[0], limiters[i])
		}
	})
}

func TestRateLimiterErrorFormat(t *testing.T) {
	rl := NewRateLimiter("test-session", 10, 0)

	_, _, err := rl.Allow()
	require.Error(t, err)

	errMsg := err.Error()
	assert.True(t, strings.Contains(errMsg, "rate limit exceeded"))
	assert.True(t, strings.Contains(errMsg, "test-session"))
	assert.True(t, strings.Contains(errMsg, "10 messages per minute"))
}
