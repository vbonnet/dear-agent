package gateway

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreaker_Defaults(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{}, testLogger)
	require.NotNil(t, cb)
	assert.Equal(t, 5, cb.failureThreshold)
	assert.Equal(t, 30*time.Second, cb.resetTimeout)
}

func TestNewCircuitBreaker_NegativeValues(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: -1,
		ResetTimeout:     -1 * time.Second,
	}, testLogger)
	assert.Equal(t, 5, cb.failureThreshold)
	assert.Equal(t, 30*time.Second, cb.resetTimeout)
}

func TestNewCircuitBreaker_CustomValues(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 10,
		ResetTimeout:     2 * time.Minute,
	}, testLogger)
	assert.Equal(t, 10, cb.failureThreshold)
	assert.Equal(t, 2*time.Minute, cb.resetTimeout)
}

func TestCircuitBreaker_NonToolCall_Passthrough(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1}, testLogger)
	mw := cb.Middleware()
	handler := mw(failHandler)

	// Non-tool calls should pass through even when the handler fails
	// (circuit breaker only applies to tools/call)
	_, err := handler(context.Background(), "initialize", initReq())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler error")

	// Circuit should still be closed for any tool (non-tool failures aren't tracked)
	assert.Equal(t, CircuitClosed, cb.State("any_tool"))
}

func TestCircuitBreaker_EmptyToolName_Passthrough(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1}, testLogger)
	mw := cb.Middleware()

	// Request with no tool name params
	emptyReq := initReq() // no CallToolParams → extractToolName returns ""
	handler := mw(successHandler)
	result, err := handler(context.Background(), "tools/call", emptyReq)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCircuitBreaker_ResetClearsAll(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: time.Minute}, testLogger)
	mw := cb.Middleware()
	handler := mw(failHandler)

	// Open circuits for two tools
	for i := 0; i < 2; i++ {
		_, _ = handler(context.Background(), "tools/call", toolCallReq("tool_x"))
		_, _ = handler(context.Background(), "tools/call", toolCallReq("tool_y"))
	}
	assert.Equal(t, CircuitOpen, cb.State("tool_x"))
	assert.Equal(t, CircuitOpen, cb.State("tool_y"))

	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.State("tool_x"))
	assert.Equal(t, CircuitClosed, cb.State("tool_y"))
}

func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     10 * time.Millisecond,
	}, testLogger)
	mw := cb.Middleware()

	// Trip the circuit
	failH := mw(failHandler)
	for i := 0; i < 2; i++ {
		_, _ = failH(context.Background(), "tools/call", toolCallReq("tool"))
	}
	assert.Equal(t, CircuitOpen, cb.State("tool"))

	// Wait for reset timeout
	time.Sleep(15 * time.Millisecond)

	// A successful call in half-open should close the circuit
	successH := mw(successHandler)
	_, err := successH(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State("tool"))
}

func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     10 * time.Millisecond,
	}, testLogger)
	mw := cb.Middleware()
	handler := mw(failHandler)

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_, _ = handler(context.Background(), "tools/call", toolCallReq("tool"))
	}
	assert.Equal(t, CircuitOpen, cb.State("tool"))

	// Wait for reset timeout to enter half-open
	time.Sleep(15 * time.Millisecond)

	// Verify it transitions to half-open
	assert.True(t, cb.canProceed("tool"))
	assert.Equal(t, CircuitHalfOpen, cb.State("tool"))

	// A failure in half-open should re-open the circuit
	_, _ = handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.Equal(t, CircuitOpen, cb.State("tool"))
}

func TestCircuitBreaker_SuccessBeforeThreshold_NoOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3}, testLogger)
	mw := cb.Middleware()

	failH := mw(failHandler)
	// 2 failures (below threshold of 3)
	for i := 0; i < 2; i++ {
		_, _ = failH(context.Background(), "tools/call", toolCallReq("tool"))
	}
	assert.Equal(t, CircuitClosed, cb.State("tool"))

	// Success resets the counter
	successH := mw(successHandler)
	_, err := successH(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)

	// Now 2 more failures shouldn't open (counter was reset)
	for i := 0; i < 2; i++ {
		_, _ = failH(context.Background(), "tools/call", toolCallReq("tool"))
	}
	assert.Equal(t, CircuitClosed, cb.State("tool"))
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 100,
		ResetTimeout:     time.Minute,
	}, testLogger)
	mw := cb.Middleware()

	var wg sync.WaitGroup
	// Run concurrent success and failure calls
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			handler := mw(successHandler)
			_, _ = handler(context.Background(), "tools/call", toolCallReq("concurrent_tool"))
		}()
		go func() {
			defer wg.Done()
			handler := mw(failHandler)
			_, _ = handler(context.Background(), "tools/call", toolCallReq("concurrent_tool"))
		}()
	}
	wg.Wait()

	// Should not panic and state should be valid
	state := cb.State("concurrent_tool")
	assert.Contains(t, []CircuitState{CircuitClosed, CircuitOpen}, state)
}

func TestCircuitBreaker_Middleware_ReturnsHandlerError(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 10}, testLogger)
	mw := cb.Middleware()

	customErr := fmt.Errorf("custom failure")
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, customErr
	})

	_, err := handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.ErrorIs(t, err, customErr)
}

func TestCircuitBreaker_Middleware_ReturnsHandlerResult(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 10}, testLogger)
	mw := cb.Middleware()

	expected := &mcp.CallToolResult{}
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return expected, nil
	})

	result, err := handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}
