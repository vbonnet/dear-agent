package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter_Defaults(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{}, testLogger)
	require.NotNil(t, rl)
	assert.Equal(t, 60, rl.defaultRate)
	assert.Equal(t, time.Minute, rl.defaultWindow)
	assert.NotNil(t, rl.toolRates)
}

func TestNewRateLimiter_NegativeValues(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		DefaultRate:   -5,
		DefaultWindow: -1 * time.Second,
	}, testLogger)
	assert.Equal(t, 60, rl.defaultRate)
	assert.Equal(t, time.Minute, rl.defaultWindow)
}

func TestNewRateLimiter_CustomValues(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		DefaultRate:   100,
		DefaultWindow: 30 * time.Second,
		ToolRates:     map[string]int{"tool_a": 10},
	}, testLogger)
	assert.Equal(t, 100, rl.defaultRate)
	assert.Equal(t, 30*time.Second, rl.defaultWindow)
	assert.Equal(t, 10, rl.toolRates["tool_a"])
}

func TestRateLimiter_EmptyToolName_Passthrough(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 1}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	// initReq() yields empty tool name
	for i := 0; i < 10; i++ {
		result, err := handler(context.Background(), "tools/call", initReq())
		assert.NoError(t, err, "request %d should pass through", i)
		assert.NotNil(t, result)
	}
}

func TestRateLimiter_Allow_Direct(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 3, DefaultWindow: time.Minute}, testLogger)

	// First 3 calls should be allowed
	for i := 0; i < 3; i++ {
		assert.True(t, rl.allow("tool"), "call %d should be allowed", i)
	}
	// 4th should be denied
	assert.False(t, rl.allow("tool"))
}

func TestRateLimiter_Allow_PerToolRate(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		DefaultRate:   100,
		DefaultWindow: time.Minute,
		ToolRates:     map[string]int{"limited_tool": 2},
	}, testLogger)

	// limited_tool gets its own lower rate
	assert.True(t, rl.allow("limited_tool"))
	assert.True(t, rl.allow("limited_tool"))
	assert.False(t, rl.allow("limited_tool"))

	// default_tool still has 100 budget
	for i := 0; i < 100; i++ {
		assert.True(t, rl.allow("default_tool"), "default_tool call %d should be allowed", i)
	}
}

func TestRateLimiter_ResetRestoredCapacity(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 2, DefaultWindow: time.Minute}, testLogger)

	assert.True(t, rl.allow("tool"))
	assert.True(t, rl.allow("tool"))
	assert.False(t, rl.allow("tool"))

	rl.Reset()

	assert.True(t, rl.allow("tool"))
	assert.True(t, rl.allow("tool"))
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	// Rate of 100/sec = high refill rate; after a brief wait we should get tokens back
	rl := NewRateLimiter(RateLimiterConfig{
		DefaultRate:   100,
		DefaultWindow: time.Second,
	}, testLogger)

	// Consume all tokens
	for i := 0; i < 100; i++ {
		rl.allow("tool")
	}
	assert.False(t, rl.allow("tool"))

	// Wait a bit for refill
	time.Sleep(50 * time.Millisecond)

	// Should have some tokens now (~5 tokens at 100/sec over 50ms)
	assert.True(t, rl.allow("tool"))
}

func TestRateLimiter_IsolatesTools(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 2, DefaultWindow: time.Minute}, testLogger)

	// Exhaust tool_a
	assert.True(t, rl.allow("tool_a"))
	assert.True(t, rl.allow("tool_a"))
	assert.False(t, rl.allow("tool_a"))

	// tool_b should still work
	assert.True(t, rl.allow("tool_b"))
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		DefaultRate:   1000,
		DefaultWindow: time.Minute,
	}, testLogger)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.allow("concurrent_tool")
		}()
	}
	wg.Wait()
	// Should not panic
}

func TestRateLimiter_Middleware_ErrorMessage(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 1, DefaultWindow: time.Minute}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	_, _ = handler(context.Background(), "tools/call", toolCallReq("my_tool"))
	_, err := handler(context.Background(), "tools/call", toolCallReq("my_tool"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Contains(t, err.Error(), "my_tool")
}
