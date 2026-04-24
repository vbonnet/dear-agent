package gateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result := expandPath("~/config/gateway.yaml")
	assert.Equal(t, filepath.Join(home, "config/gateway.yaml"), result)
}

func TestExpandPath_NoTilde(t *testing.T) {
	result := expandPath("/etc/gateway.yaml")
	assert.Equal(t, "/etc/gateway.yaml", result)
}

func TestExpandPath_Empty(t *testing.T) {
	result := expandPath("")
	assert.Equal(t, "", result)
}

func TestExpandPath_TildeOnly(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result := expandPath("~")
	assert.Equal(t, home, result)
}

func TestLoadConfig_InvalidPolicy_DefaultsToAllow(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(`
enabled: true
default_policy: "invalid_policy"
`), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, PolicyAllow, cfg.DefaultPolicy)
}

func TestLoadConfig_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(`
enabled: false
`), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	// Explicit value should be set
	assert.False(t, cfg.Enabled)
	// Defaults should remain for unset fields
	assert.Equal(t, 60, cfg.RateLimits.DefaultRate)
	assert.Equal(t, 5, cfg.CircuitBreaker.FailureThreshold)
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(``), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	// Should use defaults
	assert.True(t, cfg.Enabled)
	assert.Equal(t, PolicyAllow, cfg.DefaultPolicy)
}

func TestLoadConfig_PermissionError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(`enabled: true`), 0000)
	require.NoError(t, err)

	_, err = LoadConfig(cfgPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gateway config: read")
}

func TestLoadConfig_WithToolOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	err := os.WriteFile(cfgPath, []byte(`
rate_limits:
  default_rate: 100
  tool_overrides:
    dangerous_tool: 5
    safe_tool: 200
`), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 100, cfg.RateLimits.DefaultRate)
	assert.Equal(t, 5, cfg.RateLimits.ToolOverrides["dangerous_tool"])
	assert.Equal(t, 200, cfg.RateLimits.ToolOverrides["safe_tool"])
}

func TestToCircuitBreakerConfig_ZeroTimeout(t *testing.T) {
	cfg := &Config{
		CircuitBreaker: CBCfg{
			FailureThreshold: 3,
			ResetTimeoutSecs: 0,
		},
	}
	cbCfg := cfg.ToCircuitBreakerConfig()
	assert.Equal(t, 3, cbCfg.FailureThreshold)
	assert.Equal(t, 30*time.Second, cbCfg.ResetTimeout)
}

func TestToCircuitBreakerConfig_NegativeTimeout(t *testing.T) {
	cfg := &Config{
		CircuitBreaker: CBCfg{
			FailureThreshold: 2,
			ResetTimeoutSecs: -10,
		},
	}
	cbCfg := cfg.ToCircuitBreakerConfig()
	assert.Equal(t, 30*time.Second, cbCfg.ResetTimeout)
}

func TestToRateLimiterConfig_WithOverrides(t *testing.T) {
	cfg := &Config{
		RateLimits: RateLimitCfg{
			DefaultRate:   120,
			ToolOverrides: map[string]int{"tool_a": 10, "tool_b": 50},
		},
	}
	rlCfg := cfg.ToRateLimiterConfig()
	assert.Equal(t, 120, rlCfg.DefaultRate)
	assert.Equal(t, time.Minute, rlCfg.DefaultWindow)
	assert.Equal(t, 10, rlCfg.ToolRates["tool_a"])
	assert.Equal(t, 50, rlCfg.ToolRates["tool_b"])
}

func TestToRateLimiterConfig_NilOverrides(t *testing.T) {
	cfg := &Config{
		RateLimits: RateLimitCfg{
			DefaultRate: 60,
		},
	}
	rlCfg := cfg.ToRateLimiterConfig()
	assert.Nil(t, rlCfg.ToolRates)
}

func TestDefaultConfig_AllFieldsSet(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, PolicyAllow, cfg.DefaultPolicy)
	assert.Equal(t, 60, cfg.RateLimits.DefaultRate)
	assert.Equal(t, 5, cfg.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 30, cfg.CircuitBreaker.ResetTimeoutSecs)
	assert.True(t, cfg.Audit.Enabled)
	assert.Empty(t, cfg.Allowlist)
	assert.Empty(t, cfg.Denylist)
}
