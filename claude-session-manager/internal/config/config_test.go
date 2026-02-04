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
	os.Setenv("AGM_SESSIONS_DIR", "/tmp/env-sessions")
	os.Setenv("AGM_LOG_LEVEL", "warn")
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
log_file: ~/csm.log
lock:
  path: ~/csm.lock
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
	if cfg.LogFile != filepath.Join(homeDir, "csm.log") {
		t.Errorf("LogFile not expanded: %s", cfg.LogFile)
	}
	if cfg.Lock.Path != filepath.Join(homeDir, "csm.lock") {
		t.Errorf("Lock.Path not expanded: %s", cfg.Lock.Path)
	}
}
