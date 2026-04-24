package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sebdah/goldie/v2"
	"gopkg.in/yaml.v3"
)

// TestConfigParsing_DefaultConfig tests golden snapshot of default configuration
func TestConfigParsing_DefaultConfig(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	// Get default config
	cfg := Default()

	// Normalize paths for reproducibility (remove user-specific parts)
	cfg.SessionsDir = "~/sessions"
	cfg.Lock.Path = "/tmp/agm-1000/agm.lock"

	// Marshal to JSON for golden comparison
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-default", jsonData)
}

// TestConfigParsing_MinimalYAML tests parsing minimal YAML configuration
func TestConfigParsing_MinimalYAML(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	// Create temporary config file
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `sessions_dir: /custom/sessions
log_level: debug
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Normalize paths
	cfg.Lock.Path = "/tmp/agm-1000/agm.lock"

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-minimal-yaml", jsonData)
}

// TestConfigParsing_FullYAML tests parsing complete YAML configuration
func TestConfigParsing_FullYAML(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `sessions_dir: /tmp/test-custom/sessions
log_level: trace
log_file: /var/log/agm.log
workspace: oss
workspace_config: /tmp/test-agm/workspace.yaml
timeout:
  tmux_commands: 10s
  enabled: true
lock:
  enabled: true
  path: /tmp/agm-custom/agm.lock
health_check:
  enabled: true
  cache_duration: 10s
  probe_timeout: 5s
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-full-yaml", jsonData)
}

// TestConfigParsing_TimeoutDisabled tests configuration with disabled timeout
func TestConfigParsing_TimeoutDisabled(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `sessions_dir: /tmp/test-sessions
timeout:
  enabled: false
  tmux_commands: 0s
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg.Lock.Path = "/tmp/agm-1000/agm.lock"

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-timeout-disabled", jsonData)
}

// TestConfigParsing_LockDisabled tests configuration with disabled locking
func TestConfigParsing_LockDisabled(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `sessions_dir: /tmp/test-sessions
lock:
  enabled: false
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg.Lock.Path = "/tmp/agm-1000/agm.lock"

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-lock-disabled", jsonData)
}

// TestConfigParsing_HealthCheckCustomized tests customized health check configuration
func TestConfigParsing_HealthCheckCustomized(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `sessions_dir: /tmp/test-sessions
health_check:
  enabled: true
  cache_duration: 30s
  probe_timeout: 10s
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg.Lock.Path = "/tmp/agm-1000/agm.lock"

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-healthcheck-customized", jsonData)
}

// TestConfigParsing_WorkspaceConfig tests workspace configuration
func TestConfigParsing_WorkspaceConfig(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `sessions_dir: /tmp/test-sessions
workspace: acme
workspace_config: /tmp/test-agm/acme-workspace.yaml
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg.Lock.Path = "/tmp/agm-1000/agm.lock"

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-workspace", jsonData)
}

// TestConfigStructure_BreakingChanges tests detection of breaking config structure changes
func TestConfigStructure_BreakingChanges(t *testing.T) {
	// This test verifies the config structure matches expected golden snapshot
	// Any changes to Config struct fields will cause this test to fail,
	// alerting developers to potential breaking changes

	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	// Create a config with all fields populated to detect structure changes
	cfg := &Config{
		SessionsDir:         "~/sessions",
		LogLevel:            "info",
		LogFile:             "/var/log/agm.log",
		Workspace:           "oss",
		WorkspaceConfigPath: "~/.agm/config.yaml",
		Timeout: TimeoutConfig{
			TmuxCommands: 5 * time.Second,
			Enabled:      true,
		},
		Lock: LockConfig{
			Enabled: true,
			Path:    "/tmp/agm-1000/agm.lock",
		},
		HealthCheck: HealthCheckConfig{
			Enabled:       true,
			CacheDuration: 5 * time.Second,
			ProbeTimeout:  2 * time.Second,
		},
	}

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-structure", jsonData)
}

// TestConfigParsing_YAMLRoundTrip tests YAML -> struct -> YAML round-trip
func TestConfigParsing_YAMLRoundTrip(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	originalYAML := `sessions_dir: ~/sessions
log_level: debug
log_file: /var/log/agm.log
workspace: oss
timeout:
  tmux_commands: 8s
  enabled: true
lock:
  enabled: true
  path: /tmp/agm-1000/agm.lock
health_check:
  enabled: true
  cache_duration: 7s
  probe_timeout: 3s
`

	// Parse YAML -> struct
	var cfg Config
	if err := yaml.Unmarshal([]byte(originalYAML), &cfg); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Convert to JSON for golden comparison (more stable than YAML)
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	g.Assert(t, "config-yaml-roundtrip", jsonData)
}
