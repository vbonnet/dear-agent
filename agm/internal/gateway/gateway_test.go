package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

// helpers to create MCP-compatible request/result values

func toolCallReq(name string) mcp.Request {
	return &mcp.ServerRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: name},
	}
}

func initReq() mcp.Request {
	return &mcp.ServerRequest[*mcp.InitializeParams]{
		Params: &mcp.InitializeParams{},
	}
}

func successHandler(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
	return &mcp.CallToolResult{}, nil
}

func failHandler(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
	return nil, fmt.Errorf("handler error")
}

// --- Inspector tests ---

func TestInspector_ValidMethod(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()
	handler := mw(successHandler)

	result, err := handler(context.Background(), "tools/call", toolCallReq("test"))
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestInspector_EmptyMethod(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "", toolCallReq("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty method")
}

func TestInspector_InvalidMethod(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "tools/call; DROP TABLE", toolCallReq("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "malformed method")
}

func TestIsValidMethod(t *testing.T) {
	tests := []struct {
		method string
		valid  bool
	}{
		{"tools/call", true},
		{"tools/list", true},
		{"initialize", true},
		{"notifications/cancelled", true},
		{"$refs/resolve", true},
		{"", false},
		{"tools call", false},
		{"tools;call", false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidMethod(tt.method))
		})
	}
}

// --- Scope tests ---

func TestScope_AllowPolicy_NoLists(t *testing.T) {
	scope := NewScopeEnforcer(PolicyAllow, nil, nil, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	result, err := handler(context.Background(), "tools/call", toolCallReq("any_tool"))
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestScope_AllowPolicy_Denylist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyAllow, nil, []string{"blocked_tool"}, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("ok_tool"))
	assert.NoError(t, err)

	_, err = handler(context.Background(), "tools/call", toolCallReq("blocked_tool"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not permitted")
}

func TestScope_DenyPolicy_Allowlist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, []string{"allowed_tool"}, nil, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("allowed_tool"))
	assert.NoError(t, err)

	_, err = handler(context.Background(), "tools/call", toolCallReq("other_tool"))
	assert.Error(t, err)
}

func TestScope_NonToolCall_Passthrough(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, nil, nil, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	result, err := handler(context.Background(), "initialize", initReq())
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestScope_DenylistOverridesAllowlist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyAllow, []string{"tool"}, []string{"tool"}, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.Error(t, err)
}

// --- RateLimiter tests ---

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 10, DefaultWindow: time.Minute}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	for i := 0; i < 10; i++ {
		_, err := handler(context.Background(), "tools/call", toolCallReq("test_tool"))
		assert.NoError(t, err, "request %d should succeed", i)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 3, DefaultWindow: time.Minute}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	for i := 0; i < 3; i++ {
		_, err := handler(context.Background(), "tools/call", toolCallReq("test_tool"))
		assert.NoError(t, err)
	}

	_, err := handler(context.Background(), "tools/call", toolCallReq("test_tool"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
}

func TestRateLimiter_PerToolRates(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		DefaultRate:   100,
		DefaultWindow: time.Minute,
		ToolRates:     map[string]int{"slow_tool": 2},
	}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	for i := 0; i < 2; i++ {
		_, err := handler(context.Background(), "tools/call", toolCallReq("slow_tool"))
		assert.NoError(t, err)
	}
	_, err := handler(context.Background(), "tools/call", toolCallReq("slow_tool"))
	assert.Error(t, err)

	_, err = handler(context.Background(), "tools/call", toolCallReq("fast_tool"))
	assert.NoError(t, err)
}

func TestRateLimiter_NonToolCall_Passthrough(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 1, DefaultWindow: time.Minute}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	for i := 0; i < 10; i++ {
		_, err := handler(context.Background(), "initialize", initReq())
		assert.NoError(t, err)
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{DefaultRate: 1, DefaultWindow: time.Minute}, testLogger)
	mw := rl.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)
	_, err = handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.Error(t, err)

	rl.Reset()
	_, err = handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)
}

// --- CircuitBreaker tests ---

func TestCircuitBreaker_ClosedByDefault(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3}, testLogger)
	assert.Equal(t, CircuitClosed, cb.State("tool"))
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: time.Minute}, testLogger)
	mw := cb.Middleware()
	handler := mw(failHandler)

	for i := 0; i < 3; i++ {
		_, _ = handler(context.Background(), "tools/call", toolCallReq("flaky_tool"))
	}

	assert.Equal(t, CircuitOpen, cb.State("flaky_tool"))

	_, err := handler(context.Background(), "tools/call", toolCallReq("flaky_tool"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3}, testLogger)
	mw := cb.Middleware()

	failH := mw(failHandler)
	for i := 0; i < 2; i++ {
		_, _ = failH(context.Background(), "tools/call", toolCallReq("tool"))
	}
	assert.Equal(t, CircuitClosed, cb.State("tool"))

	successH := mw(successHandler)
	_, err := successH(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State("tool"))
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 10 * time.Millisecond}, testLogger)
	mw := cb.Middleware()
	handler := mw(failHandler)

	for i := 0; i < 2; i++ {
		_, _ = handler(context.Background(), "tools/call", toolCallReq("tool"))
	}
	assert.Equal(t, CircuitOpen, cb.State("tool"))

	time.Sleep(15 * time.Millisecond)

	assert.True(t, cb.canProceed("tool"))
	assert.Equal(t, CircuitHalfOpen, cb.State("tool"))
}

func TestCircuitBreaker_IsolatesTools(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2}, testLogger)
	mw := cb.Middleware()
	handler := mw(failHandler)

	for i := 0; i < 2; i++ {
		_, _ = handler(context.Background(), "tools/call", toolCallReq("tool_a"))
	}
	assert.Equal(t, CircuitOpen, cb.State("tool_a"))
	assert.Equal(t, CircuitClosed, cb.State("tool_b"))

	successH := mw(successHandler)
	_, err := successH(context.Background(), "tools/call", toolCallReq("tool_b"))
	assert.NoError(t, err)
}

// --- AuditLogger tests ---

func TestAuditLogger_LogsSuccess(t *testing.T) {
	audit := NewAuditLogger(testLogger)
	mw := audit.Middleware()
	handler := mw(successHandler)

	result, err := handler(context.Background(), "tools/call", toolCallReq("test_tool"))
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAuditLogger_LogsError(t *testing.T) {
	audit := NewAuditLogger(testLogger)
	mw := audit.Middleware()
	handler := mw(failHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("test_tool"))
	assert.Error(t, err)
}

func TestAuditLogger_NonToolMethod(t *testing.T) {
	audit := NewAuditLogger(testLogger)
	mw := audit.Middleware()
	handler := mw(successHandler)

	result, err := handler(context.Background(), "initialize", initReq())
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// --- Config tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, PolicyAllow, cfg.DefaultPolicy)
	assert.Equal(t, 60, cfg.RateLimits.DefaultRate)
	assert.Equal(t, 5, cfg.CircuitBreaker.FailureThreshold)
	assert.True(t, cfg.Audit.Enabled)
}

func TestLoadConfig_Missing(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/gateway.yaml")
	assert.NoError(t, err)
	assert.True(t, cfg.Enabled)
}

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(`
enabled: true
default_policy: deny
allowlist:
  - tool_a
  - tool_b
denylist:
  - tool_c
rate_limits:
  default_rate: 30
  tool_overrides:
    tool_a: 10
circuit_breaker:
  failure_threshold: 3
  reset_timeout_secs: 60
audit:
  enabled: true
`), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, PolicyDeny, cfg.DefaultPolicy)
	assert.Equal(t, []string{"tool_a", "tool_b"}, cfg.Allowlist)
	assert.Equal(t, []string{"tool_c"}, cfg.Denylist)
	assert.Equal(t, 30, cfg.RateLimits.DefaultRate)
	assert.Equal(t, 10, cfg.RateLimits.ToolOverrides["tool_a"])
	assert.Equal(t, 3, cfg.CircuitBreaker.FailureThreshold)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(`{{{invalid`), 0644)
	require.NoError(t, err)

	_, err = LoadConfig(cfgPath)
	assert.Error(t, err)
}

func TestConfig_ToRateLimiterConfig(t *testing.T) {
	cfg := DefaultConfig()
	rlCfg := cfg.ToRateLimiterConfig()
	assert.Equal(t, 60, rlCfg.DefaultRate)
	assert.Equal(t, time.Minute, rlCfg.DefaultWindow)
}

func TestConfig_ToCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultConfig()
	cbCfg := cfg.ToCircuitBreakerConfig()
	assert.Equal(t, 5, cbCfg.FailureThreshold)
	assert.Equal(t, 30*time.Second, cbCfg.ResetTimeout)
}

// --- Gateway integration test ---

func TestGateway_New(t *testing.T) {
	cfg := DefaultConfig()
	gw := New(cfg, testLogger)

	assert.NotNil(t, gw.Inspector)
	assert.NotNil(t, gw.Scope)
	assert.NotNil(t, gw.RateLimiter)
	assert.NotNil(t, gw.CircuitBreaker)
	assert.NotNil(t, gw.AuditLogger)
}

func TestGateway_NilConfig(t *testing.T) {
	gw := New(nil, nil)
	assert.NotNil(t, gw)
	assert.NotNil(t, gw.Config)
}

func TestGateway_Install(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test",
		Version: "1.0.0",
	}, nil)

	cfg := DefaultConfig()
	gw := New(cfg, testLogger)
	gw.Install(server)
}

// --- CircuitState String tests ---

func TestCircuitState_String(t *testing.T) {
	assert.Equal(t, "closed", CircuitClosed.String())
	assert.Equal(t, "open", CircuitOpen.String())
	assert.Equal(t, "half-open", CircuitHalfOpen.String())
	assert.Equal(t, "unknown", CircuitState(99).String())
}
