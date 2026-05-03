package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/config"
	"gopkg.in/yaml.v3"
)

// TestSetTelemetryEnabled_Enable tests enabling telemetry
func TestSetTelemetryEnabled_Enable(t *testing.T) {
	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Enable telemetry
	if err := setTelemetryEnabled(true); err != nil {
		t.Fatalf("Failed to enable telemetry: %v", err)
	}

	// Verify config file was created
	configPath := filepath.Join(tmpHome, ".engram", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Read and verify config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if !cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=true, got false")
	}
}

// TestSetTelemetryEnabled_Disable tests disabling telemetry
func TestSetTelemetryEnabled_Disable(t *testing.T) {
	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Disable telemetry
	if err := setTelemetryEnabled(false); err != nil {
		t.Fatalf("Failed to disable telemetry: %v", err)
	}

	// Verify config file was created
	configPath := filepath.Join(tmpHome, ".engram", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Read and verify config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=false, got true")
	}
}

// TestSetTelemetryEnabled_UpdateExisting tests updating existing config
func TestSetTelemetryEnabled_UpdateExisting(t *testing.T) {
	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create .engram directory
	engramDir := filepath.Join(tmpHome, ".engram")
	if err := os.MkdirAll(engramDir, 0755); err != nil {
		t.Fatalf("Failed to create .engram directory: %v", err)
	}

	// Create initial config with telemetry enabled and other settings
	configPath := filepath.Join(engramDir, "config.yaml")
	initialConfig := config.Config{
		Platform: config.PlatformConfig{
			Agent:       "claude-code",
			EngramPath:  "~/engrams",
			TokenBudget: 10000,
		},
		Telemetry: config.TelemetryConfig{
			Enabled: true,
			Path:    "~/telemetry",
		},
	}

	data, err := yaml.Marshal(&initialConfig)
	if err != nil {
		t.Fatalf("Failed to marshal initial config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// Disable telemetry
	if err := setTelemetryEnabled(false); err != nil {
		t.Fatalf("Failed to disable telemetry: %v", err)
	}

	// Read and verify config
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	// Telemetry should be disabled
	if cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=false, got true")
	}

	// Other settings should be preserved
	if cfg.Platform.Agent != "claude-code" {
		t.Errorf("Expected agent='claude-code', got '%s'", cfg.Platform.Agent)
	}
	if cfg.Platform.EngramPath != "~/engrams" {
		t.Errorf("Expected engram_path='~/engrams', got '%s'", cfg.Platform.EngramPath)
	}
	if cfg.Platform.TokenBudget != 10000 {
		t.Errorf("Expected token_budget=10000, got %d", cfg.Platform.TokenBudget)
	}
	if cfg.Telemetry.Path != "~/telemetry" {
		t.Errorf("Expected telemetry.path='~/telemetry', got '%s'", cfg.Telemetry.Path)
	}
}

// TestSetTelemetryEnabled_Toggle tests toggling telemetry on/off
func TestSetTelemetryEnabled_Toggle(t *testing.T) {
	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Enable
	if err := setTelemetryEnabled(true); err != nil {
		t.Fatalf("Failed to enable: %v", err)
	}

	configPath := filepath.Join(tmpHome, ".engram", "config.yaml")

	// Verify enabled
	data, _ := os.ReadFile(configPath)
	var cfg config.Config
	yaml.Unmarshal(data, &cfg)
	if !cfg.Telemetry.Enabled {
		t.Error("Expected enabled=true after enable")
	}

	// Disable
	if err := setTelemetryEnabled(false); err != nil {
		t.Fatalf("Failed to disable: %v", err)
	}

	// Verify disabled
	data, _ = os.ReadFile(configPath)
	yaml.Unmarshal(data, &cfg)
	if cfg.Telemetry.Enabled {
		t.Error("Expected enabled=false after disable")
	}

	// Re-enable
	if err := setTelemetryEnabled(true); err != nil {
		t.Fatalf("Failed to re-enable: %v", err)
	}

	// Verify re-enabled
	data, _ = os.ReadFile(configPath)
	yaml.Unmarshal(data, &cfg)
	if !cfg.Telemetry.Enabled {
		t.Error("Expected enabled=true after re-enable")
	}
}

// TestSetTelemetryEnabled_CreatesDirectory tests directory creation
func TestSetTelemetryEnabled_CreatesDirectory(t *testing.T) {
	// Create temporary home directory (but NOT .engram subdirectory)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Verify .engram doesn't exist yet
	engramDir := filepath.Join(tmpHome, ".engram")
	if _, err := os.Stat(engramDir); !os.IsNotExist(err) {
		t.Fatal(".engram directory already exists (test setup error)")
	}

	// Enable telemetry (should create directory)
	if err := setTelemetryEnabled(true); err != nil {
		t.Fatalf("Failed to enable telemetry: %v", err)
	}

	// Verify .engram directory was created
	if _, err := os.Stat(engramDir); os.IsNotExist(err) {
		t.Error(".engram directory was not created")
	}

	// Verify config file exists
	configPath := filepath.Join(engramDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.yaml was not created")
	}
}

// TestSetTelemetryEnabled_InvalidYAML tests handling of corrupted config
func TestSetTelemetryEnabled_InvalidYAML(t *testing.T) {
	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create .engram directory
	engramDir := filepath.Join(tmpHome, ".engram")
	if err := os.MkdirAll(engramDir, 0755); err != nil {
		t.Fatalf("Failed to create .engram directory: %v", err)
	}

	// Write invalid YAML
	configPath := filepath.Join(engramDir, "config.yaml")
	invalidYAML := "this is not valid: yaml: content:\n  - broken"
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write invalid YAML: %v", err)
	}

	// Attempt to enable telemetry (should fail due to invalid YAML)
	err := setTelemetryEnabled(true)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

// TestSetTelemetryEnabled_PermissionDenied tests handling of permission errors
func TestSetTelemetryEnabled_PermissionDenied(t *testing.T) {
	// Skip on Windows (different permission model)
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("Skipping permission test on Windows")
	}
	// Skip when running as root (root bypasses Unix permission checks)
	if os.Getuid() == 0 {
		t.Skip("Skipping test: requires non-root user for filesystem permission checks")
	}

	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create .engram directory with no write permissions
	engramDir := filepath.Join(tmpHome, ".engram")
	if err := os.MkdirAll(engramDir, 0555); err != nil {
		t.Fatalf("Failed to create .engram directory: %v", err)
	}
	defer os.Chmod(engramDir, 0755) // Restore permissions for cleanup

	// Attempt to enable telemetry (should fail due to permissions)
	err := setTelemetryEnabled(true)
	if err == nil {
		t.Error("Expected error for permission denied, got nil")
	}
}

// TestSetTelemetryEnabled_FilePermissions tests that config file has correct permissions
// P0-1: User config file should be 0600 (owner read/write only)
func TestSetTelemetryEnabled_FilePermissions(t *testing.T) {
	// Skip on Windows (different permission model)
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("Skipping permission test on Windows")
	}

	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Enable telemetry
	if err := setTelemetryEnabled(true); err != nil {
		t.Fatalf("Failed to enable telemetry: %v", err)
	}

	// Check file permissions
	configPath := filepath.Join(tmpHome, ".engram", "config.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	mode := info.Mode()
	if mode.Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %#o", mode.Perm())
		t.Error("P0-1 SECURITY ISSUE: User config file is world-readable")
	}
}

// TestSetTelemetryEnabled_PreservesComments tests that comments are not preserved
// (This is expected behavior - YAML library doesn't preserve comments)
func TestSetTelemetryEnabled_YAMLFormatting(t *testing.T) {
	// Create temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create .engram directory
	engramDir := filepath.Join(tmpHome, ".engram")
	if err := os.MkdirAll(engramDir, 0755); err != nil {
		t.Fatalf("Failed to create .engram directory: %v", err)
	}

	// Create config with comments
	configPath := filepath.Join(engramDir, "config.yaml")
	configWithComments := `# User configuration
platform:
  agent: claude-code  # My preferred agent
  engram_path: ~/engrams
telemetry:
  enabled: true  # Telemetry enabled
`
	if err := os.WriteFile(configPath, []byte(configWithComments), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Update telemetry
	if err := setTelemetryEnabled(false); err != nil {
		t.Fatalf("Failed to disable telemetry: %v", err)
	}

	// Read config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	// Verify it's valid YAML (comments will be lost)
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse updated config: %v", err)
	}

	// Verify values are preserved
	if cfg.Platform.Agent != "claude-code" {
		t.Errorf("Expected agent='claude-code', got '%s'", cfg.Platform.Agent)
	}
	if cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=false, got true")
	}
}
