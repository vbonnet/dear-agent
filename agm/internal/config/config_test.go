package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	// Check existing fields
	if cfg.SessionsDir == "" {
		t.Error("SessionsDir is empty")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %s, expected 'info'", cfg.LogLevel)
	}

	// Check timeout config
	if cfg.Timeout.TmuxCommands != 5*time.Second {
		t.Errorf("Timeout.TmuxCommands = %v, expected 5s", cfg.Timeout.TmuxCommands)
	}
	if !cfg.Timeout.Enabled {
		t.Error("Timeout.Enabled should be true by default")
	}

	// Check lock config
	if !cfg.Lock.Enabled {
		t.Error("Lock.Enabled should be true by default")
	}
	if cfg.Lock.Path == "" {
		t.Error("Lock.Path is empty")
	}

	// Check health check config
	if !cfg.HealthCheck.Enabled {
		t.Error("HealthCheck.Enabled should be true by default")
	}
	if cfg.HealthCheck.CacheDuration != 5*time.Second {
		t.Errorf("HealthCheck.CacheDuration = %v, expected 5s", cfg.HealthCheck.CacheDuration)
	}
	if cfg.HealthCheck.ProbeTimeout != 2*time.Second {
		t.Errorf("HealthCheck.ProbeTimeout = %v, expected 2s", cfg.HealthCheck.ProbeTimeout)
	}

	// Check adapters config
	if cfg.Adapters.OpenCode.Enabled {
		t.Error("Adapters.OpenCode.Enabled should be false by default (opt-in)")
	}
	if cfg.Adapters.OpenCode.ServerURL != "http://localhost:4096" {
		t.Errorf("Adapters.OpenCode.ServerURL = %s, expected http://localhost:4096", cfg.Adapters.OpenCode.ServerURL)
	}
	if cfg.Adapters.OpenCode.Reconnect.InitialDelay != 1*time.Second {
		t.Errorf("Adapters.OpenCode.Reconnect.InitialDelay = %v, expected 1s", cfg.Adapters.OpenCode.Reconnect.InitialDelay)
	}
	if cfg.Adapters.OpenCode.Reconnect.MaxDelay != 30*time.Second {
		t.Errorf("Adapters.OpenCode.Reconnect.MaxDelay = %v, expected 30s", cfg.Adapters.OpenCode.Reconnect.MaxDelay)
	}
	if cfg.Adapters.OpenCode.Reconnect.Multiplier != 2 {
		t.Errorf("Adapters.OpenCode.Reconnect.Multiplier = %d, expected 2", cfg.Adapters.OpenCode.Reconnect.Multiplier)
	}
	if !cfg.Adapters.OpenCode.FallbackTmux {
		t.Error("Adapters.OpenCode.FallbackTmux should be true by default")
	}

	// Check Claude hooks
	if cfg.Adapters.ClaudeHooks.Enabled {
		t.Error("Adapters.ClaudeHooks.Enabled should be false by default")
	}
	if cfg.Adapters.ClaudeHooks.ListenAddr != "127.0.0.1:14321" {
		t.Errorf("Adapters.ClaudeHooks.ListenAddr = %s, expected 127.0.0.1:14321", cfg.Adapters.ClaudeHooks.ListenAddr)
	}

	// Check Gemini hooks
	if cfg.Adapters.GeminiHooks.Enabled {
		t.Error("Adapters.GeminiHooks.Enabled should be false by default")
	}
	if cfg.Adapters.GeminiHooks.SocketPath != "/tmp/agm-gemini-hook.sock" {
		t.Errorf("Adapters.GeminiHooks.SocketPath = %s, expected /tmp/agm-gemini-hook.sock", cfg.Adapters.GeminiHooks.SocketPath)
	}
}

func TestLoad_DefaultsWhenMissing(t *testing.T) {
	// Load non-existent file
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	// Should have default values
	if cfg.Timeout.TmuxCommands != 5*time.Second {
		t.Errorf("Default timeout not applied: %v", cfg.Timeout.TmuxCommands)
	}
}

func TestLoad_YAMLParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write test config
	configContent := `sessions_dir: /tmp/sessions
log_level: debug
timeout:
  tmux_commands: 10s
  enabled: true
lock:
  enabled: true
  path: /tmp/test.lock
health_check:
  enabled: false
  cache_duration: 3s
  probe_timeout: 1s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify values
	if cfg.SessionsDir != "/tmp/sessions" {
		t.Errorf("SessionsDir = %s, expected /tmp/sessions", cfg.SessionsDir)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, expected debug", cfg.LogLevel)
	}
	if cfg.Timeout.TmuxCommands != 10*time.Second {
		t.Errorf("Timeout.TmuxCommands = %v, expected 10s", cfg.Timeout.TmuxCommands)
	}
	if cfg.Timeout.Enabled != true {
		t.Error("Timeout.Enabled should be true")
	}
	if cfg.Lock.Path != "/tmp/test.lock" {
		t.Errorf("Lock.Path = %s, expected /tmp/test.lock", cfg.Lock.Path)
	}
	if cfg.HealthCheck.Enabled {
		t.Error("HealthCheck.Enabled should be false")
	}
	if cfg.HealthCheck.CacheDuration != 3*time.Second {
		t.Errorf("HealthCheck.CacheDuration = %v, expected 3s", cfg.HealthCheck.CacheDuration)
	}
}

func TestLoad_PartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write partial config (only override some fields)
	configContent := `sessions_dir: /tmp/sessions
timeout:
  tmux_commands: 15s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should have custom timeout
	if cfg.Timeout.TmuxCommands != 15*time.Second {
		t.Errorf("Timeout.TmuxCommands = %v, expected 15s", cfg.Timeout.TmuxCommands)
	}

	// Should still have default health check values
	if cfg.HealthCheck.CacheDuration != 5*time.Second {
		t.Errorf("HealthCheck.CacheDuration = %v, expected default 5s", cfg.HealthCheck.CacheDuration)
	}

	// Should have default lock enabled
	if !cfg.Lock.Enabled {
		t.Error("Lock.Enabled should be true by default")
	}
}

func TestLoad_EnvironmentOverrides(t *testing.T) {
	// Set environment variables
	t.Setenv("AGM_SESSIONS_DIR", "/tmp/env-sessions")
	t.Setenv("AGM_LOG_LEVEL", "warn")
	defer os.Unsetenv("AGM_SESSIONS_DIR")
	defer os.Unsetenv("AGM_LOG_LEVEL")

	// Load config
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Environment should override defaults
	if cfg.SessionsDir != "/tmp/env-sessions" {
		t.Errorf("SessionsDir = %s, expected /tmp/env-sessions", cfg.SessionsDir)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %s, expected warn", cfg.LogLevel)
	}
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHome bool
	}{
		{"absolute path", "/tmp/test", false},
		{"tilde only", "~", true},
		{"tilde with path", "~/sessions", true},
		{"relative path", "sessions", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandHome(tt.input)

			if tt.wantHome {
				homeDir, _ := os.UserHomeDir()
				if result == tt.input {
					t.Errorf("expandHome(%s) = %s, expected expansion", tt.input, result)
				}
				if tt.input == "~" && result != homeDir {
					t.Errorf("expandHome(~) = %s, expected %s", result, homeDir)
				}
			} else {
				if result != tt.input {
					t.Errorf("expandHome(%s) = %s, should not expand", tt.input, result)
				}
			}
		})
	}
}

func TestLoad_ExpandHomePaths(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with tilde paths
	configContent := `sessions_dir: ~/my-sessions
log_file: ~/agm.log
lock:
  path: ~/agm.lock
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	homeDir, _ := os.UserHomeDir()

	// All paths should be expanded
	if cfg.SessionsDir != filepath.Join(homeDir, "my-sessions") {
		t.Errorf("SessionsDir not expanded: %s", cfg.SessionsDir)
	}
	if cfg.LogFile != filepath.Join(homeDir, "agm.log") {
		t.Errorf("LogFile not expanded: %s", cfg.LogFile)
	}
	if cfg.Lock.Path != filepath.Join(homeDir, "agm.lock") {
		t.Errorf("Lock.Path not expanded: %s", cfg.Lock.Path)
	}
}

// ============================================================================
// OpenCode Adapter Configuration Tests (Task 1.5)
// ============================================================================

func TestLoad_OpenCodeAdapterConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with OpenCode adapter settings
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:8080"
    reconnect:
      initial_delay: 2s
      max_delay: 60s
      multiplier: 3
    fallback_to_tmux: false
  claude_hooks:
    enabled: true
    listen_addr: "0.0.0.0:9999"
  gemini_hooks:
    enabled: true
    socket_path: "/tmp/custom.sock"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify OpenCode adapter
	if !cfg.Adapters.OpenCode.Enabled {
		t.Error("Adapters.OpenCode.Enabled should be true")
	}
	if cfg.Adapters.OpenCode.ServerURL != "http://localhost:8080" {
		t.Errorf("Adapters.OpenCode.ServerURL = %s, expected http://localhost:8080", cfg.Adapters.OpenCode.ServerURL)
	}
	if cfg.Adapters.OpenCode.Reconnect.InitialDelay != 2*time.Second {
		t.Errorf("Adapters.OpenCode.Reconnect.InitialDelay = %v, expected 2s", cfg.Adapters.OpenCode.Reconnect.InitialDelay)
	}
	if cfg.Adapters.OpenCode.Reconnect.MaxDelay != 60*time.Second {
		t.Errorf("Adapters.OpenCode.Reconnect.MaxDelay = %v, expected 60s", cfg.Adapters.OpenCode.Reconnect.MaxDelay)
	}
	if cfg.Adapters.OpenCode.Reconnect.Multiplier != 3 {
		t.Errorf("Adapters.OpenCode.Reconnect.Multiplier = %d, expected 3", cfg.Adapters.OpenCode.Reconnect.Multiplier)
	}
	if cfg.Adapters.OpenCode.FallbackTmux {
		t.Error("Adapters.OpenCode.FallbackTmux should be false")
	}

	// Verify Claude hooks
	if !cfg.Adapters.ClaudeHooks.Enabled {
		t.Error("Adapters.ClaudeHooks.Enabled should be true")
	}
	if cfg.Adapters.ClaudeHooks.ListenAddr != "0.0.0.0:9999" {
		t.Errorf("Adapters.ClaudeHooks.ListenAddr = %s, expected 0.0.0.0:9999", cfg.Adapters.ClaudeHooks.ListenAddr)
	}

	// Verify Gemini hooks
	if !cfg.Adapters.GeminiHooks.Enabled {
		t.Error("Adapters.GeminiHooks.Enabled should be true")
	}
	if cfg.Adapters.GeminiHooks.SocketPath != "/tmp/custom.sock" {
		t.Errorf("Adapters.GeminiHooks.SocketPath = %s, expected /tmp/custom.sock", cfg.Adapters.GeminiHooks.SocketPath)
	}
}

func TestLoad_PartialOpenCodeConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write partial config (only override enabled and server_url)
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:9000"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should have custom enabled and server_url
	if !cfg.Adapters.OpenCode.Enabled {
		t.Error("Adapters.OpenCode.Enabled should be true")
	}
	if cfg.Adapters.OpenCode.ServerURL != "http://localhost:9000" {
		t.Errorf("Adapters.OpenCode.ServerURL = %s, expected http://localhost:9000", cfg.Adapters.OpenCode.ServerURL)
	}

	// Should still have default reconnect values
	if cfg.Adapters.OpenCode.Reconnect.InitialDelay != 1*time.Second {
		t.Errorf("Adapters.OpenCode.Reconnect.InitialDelay = %v, expected default 1s", cfg.Adapters.OpenCode.Reconnect.InitialDelay)
	}
	if cfg.Adapters.OpenCode.Reconnect.MaxDelay != 30*time.Second {
		t.Errorf("Adapters.OpenCode.Reconnect.MaxDelay = %v, expected default 30s", cfg.Adapters.OpenCode.Reconnect.MaxDelay)
	}
	if cfg.Adapters.OpenCode.Reconnect.Multiplier != 2 {
		t.Errorf("Adapters.OpenCode.Reconnect.Multiplier = %d, expected default 2", cfg.Adapters.OpenCode.Reconnect.Multiplier)
	}
	if !cfg.Adapters.OpenCode.FallbackTmux {
		t.Error("Adapters.OpenCode.FallbackTmux should be true by default")
	}
}

func TestLoad_OpenCodeEnvironmentOverrides(t *testing.T) {
	// Set environment variables
	t.Setenv("OPENCODE_SERVER_URL", "http://localhost:7777")
	t.Setenv("OPENCODE_ADAPTER_ENABLED", "true")
	defer os.Unsetenv("OPENCODE_SERVER_URL")
	defer os.Unsetenv("OPENCODE_ADAPTER_ENABLED")

	// Load config with no file
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Environment should override defaults
	if cfg.Adapters.OpenCode.ServerURL != "http://localhost:7777" {
		t.Errorf("Adapters.OpenCode.ServerURL = %s, expected http://localhost:7777", cfg.Adapters.OpenCode.ServerURL)
	}
	if !cfg.Adapters.OpenCode.Enabled {
		t.Error("Adapters.OpenCode.Enabled should be true from env")
	}
}

func TestLoad_OpenCodeEnvironmentEnabled_BooleanVariants(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"true", "true", true},
		{"1", "1", true},
		{"false", "false", false},
		{"0", "0", false},
		{"empty", "", false},
		{"invalid", "yes", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("OPENCODE_ADAPTER_ENABLED", tt.envValue)
				defer os.Unsetenv("OPENCODE_ADAPTER_ENABLED")
			}

			cfg, err := Load("/nonexistent/config.yaml")
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}

			if cfg.Adapters.OpenCode.Enabled != tt.expected {
				t.Errorf("OPENCODE_ADAPTER_ENABLED=%s: got %v, expected %v", tt.envValue, cfg.Adapters.OpenCode.Enabled, tt.expected)
			}
		})
	}
}

func TestLoad_OpenCodeEnvironmentOverridesFileConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config file
	configContent := `adapters:
  opencode:
    enabled: false
    server_url: "http://localhost:4096"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables (should override file)
	t.Setenv("OPENCODE_SERVER_URL", "http://localhost:8888")
	t.Setenv("OPENCODE_ADAPTER_ENABLED", "true")
	defer os.Unsetenv("OPENCODE_SERVER_URL")
	defer os.Unsetenv("OPENCODE_ADAPTER_ENABLED")

	// Load config
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Environment should override file
	if cfg.Adapters.OpenCode.ServerURL != "http://localhost:8888" {
		t.Errorf("ServerURL = %s, expected http://localhost:8888 (env override)", cfg.Adapters.OpenCode.ServerURL)
	}
	if !cfg.Adapters.OpenCode.Enabled {
		t.Error("Enabled should be true from env (overriding file false)")
	}
}

func TestValidate_OpenCodeEnabled_RequiresServerURL(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with enabled but empty server_url
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: ""
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail validation
	_, err := Load(configFile)
	if err == nil {
		t.Fatal("Load() should fail when enabled=true but server_url is empty")
	}
	if err.Error() != "config validation failed: adapters.opencode.server_url is required when enabled" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestValidate_OpenCodeReconnect_InitialDelayPositive(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with invalid initial_delay
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 0s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail validation
	_, err := Load(configFile)
	if err == nil {
		t.Fatal("Load() should fail when initial_delay <= 0")
	}
	if err.Error() != "config validation failed: adapters.opencode.reconnect.initial_delay must be > 0" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestValidate_OpenCodeReconnect_MaxDelayPositive(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with invalid max_delay
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 1s
      max_delay: 0s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail validation
	_, err := Load(configFile)
	if err == nil {
		t.Fatal("Load() should fail when max_delay <= 0")
	}
	if err.Error() != "config validation failed: adapters.opencode.reconnect.max_delay must be > 0" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestValidate_OpenCodeReconnect_MaxDelayGreaterThanInitial(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with max_delay < initial_delay
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 30s
      max_delay: 10s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail validation
	_, err := Load(configFile)
	if err == nil {
		t.Fatal("Load() should fail when max_delay < initial_delay")
	}
	if err.Error() != "config validation failed: adapters.opencode.reconnect.max_delay must be >= initial_delay" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestValidate_OpenCodeReconnect_MultiplierValid(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with invalid multiplier
	configContent := `adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 1s
      max_delay: 30s
      multiplier: 0
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail validation
	_, err := Load(configFile)
	if err == nil {
		t.Fatal("Load() should fail when multiplier < 1")
	}
	if err.Error() != "config validation failed: adapters.opencode.reconnect.multiplier must be >= 1" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestValidate_OpenCodeDisabled_SkipsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write config with invalid values but disabled
	configContent := `adapters:
  opencode:
    enabled: false
    server_url: ""
    reconnect:
      initial_delay: 0s
      max_delay: 0s
      multiplier: 0
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should succeed (validation skipped when disabled)
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() should succeed when adapter disabled: %v", err)
	}

	if cfg.Adapters.OpenCode.Enabled {
		t.Error("Adapters.OpenCode.Enabled should be false")
	}
}

func TestLoad_BackwardCompatibility_NoAdaptersSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Write old config without adapters section
	configContent := `sessions_dir: ~/sessions
log_level: info
timeout:
  tmux_commands: 5s
  enabled: true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should succeed with defaults
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should have default adapter config
	if cfg.Adapters.OpenCode.Enabled {
		t.Error("Adapters.OpenCode.Enabled should be false by default")
	}
	if cfg.Adapters.OpenCode.ServerURL != "http://localhost:4096" {
		t.Errorf("Adapters.OpenCode.ServerURL = %s, expected default", cfg.Adapters.OpenCode.ServerURL)
	}
}
