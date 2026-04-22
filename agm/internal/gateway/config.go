package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the gateway configuration loaded from gateway.yaml.
type Config struct {
	Enabled       bool          `yaml:"enabled"`
	DefaultPolicy ScopePolicy   `yaml:"default_policy"` // "allow" or "deny"
	Allowlist     []string      `yaml:"allowlist"`
	Denylist      []string      `yaml:"denylist"`
	RateLimits    RateLimitCfg  `yaml:"rate_limits"`
	CircuitBreaker CBCfg        `yaml:"circuit_breaker"`
	Audit         AuditCfg      `yaml:"audit"`
}

// RateLimitCfg holds rate limit settings from config.
type RateLimitCfg struct {
	DefaultRate   int            `yaml:"default_rate"`   // requests per minute
	ToolOverrides map[string]int `yaml:"tool_overrides"` // per-tool overrides
}

// CBCfg holds circuit breaker settings from config.
type CBCfg struct {
	FailureThreshold int `yaml:"failure_threshold"`
	ResetTimeoutSecs int `yaml:"reset_timeout_secs"`
}

// AuditCfg holds audit logging settings from config.
type AuditCfg struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		DefaultPolicy: PolicyAllow,
		RateLimits: RateLimitCfg{
			DefaultRate: 60,
		},
		CircuitBreaker: CBCfg{
			FailureThreshold: 5,
			ResetTimeoutSecs: 30,
		},
		Audit: AuditCfg{
			Enabled: true,
		},
	}
}

// LoadConfig loads gateway configuration from a YAML file.
// Falls back to defaults if the file doesn't exist.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	path = expandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("gateway config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("gateway config: parse %s: %w", path, err)
	}

	// Validate policy
	if cfg.DefaultPolicy != PolicyAllow && cfg.DefaultPolicy != PolicyDeny {
		cfg.DefaultPolicy = PolicyAllow
	}

	return cfg, nil
}

// ToRateLimiterConfig converts config to RateLimiterConfig.
func (c *Config) ToRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		DefaultRate:   c.RateLimits.DefaultRate,
		DefaultWindow: time.Minute,
		ToolRates:     c.RateLimits.ToolOverrides,
	}
}

// ToCircuitBreakerConfig converts config to CircuitBreakerConfig.
func (c *Config) ToCircuitBreakerConfig() CircuitBreakerConfig {
	timeout := time.Duration(c.CircuitBreaker.ResetTimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return CircuitBreakerConfig{
		FailureThreshold: c.CircuitBreaker.FailureThreshold,
		ResetTimeout:     timeout,
	}
}

func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
