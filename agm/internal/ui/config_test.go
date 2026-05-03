package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Verify defaults
	if !cfg.Defaults.Interactive {
		t.Error("Interactive should default to true")
	}
	if !cfg.Defaults.AutoAssociateUUID {
		t.Error("AutoAssociateUUID should default to true")
	}
	if !cfg.Defaults.ConfirmDestructive {
		t.Error("ConfirmDestructive should default to true")
	}
	if cfg.Defaults.CleanupThresholdDays != 30 {
		t.Errorf("CleanupThresholdDays = %d, want 30", cfg.Defaults.CleanupThresholdDays)
	}
	if cfg.Defaults.ArchiveThresholdDays != 90 {
		t.Errorf("ArchiveThresholdDays = %d, want 90", cfg.Defaults.ArchiveThresholdDays)
	}

	// UI defaults
	if cfg.UI.Theme != "agm" {
		t.Errorf("Theme = %q, want %q", cfg.UI.Theme, "agm")
	}
	if cfg.UI.PickerHeight != 15 {
		t.Errorf("PickerHeight = %d, want 15", cfg.UI.PickerHeight)
	}
	if !cfg.UI.ShowProjectPaths {
		t.Error("ShowProjectPaths should default to true")
	}
	if !cfg.UI.ShowTags {
		t.Error("ShowTags should default to true")
	}
	if !cfg.UI.FuzzySearch {
		t.Error("FuzzySearch should default to true")
	}
	if cfg.UI.NoColor {
		t.Error("NoColor should default to false")
	}
	if cfg.UI.ScreenReader {
		t.Error("ScreenReader should default to false")
	}

	// Advanced defaults
	if cfg.Advanced.TmuxTimeout != "5s" {
		t.Errorf("TmuxTimeout = %q, want %q", cfg.Advanced.TmuxTimeout, "5s")
	}
	if cfg.Advanced.HealthCheckCache != "5s" {
		t.Errorf("HealthCheckCache = %q, want %q", cfg.Advanced.HealthCheckCache, "5s")
	}
	if cfg.Advanced.LockTimeout != "30s" {
		t.Errorf("LockTimeout = %q, want %q", cfg.Advanced.LockTimeout, "30s")
	}
	if cfg.Advanced.UUIDDetectionWindow != "5m" {
		t.Errorf("UUIDDetectionWindow = %q, want %q", cfg.Advanced.UUIDDetectionWindow, "5m")
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	// Set HOME to a temp dir with no config file
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := LoadConfig()

	// Should return defaults
	defaults := DefaultConfig()
	if cfg.UI.Theme != defaults.UI.Theme {
		t.Errorf("Theme = %q, want default %q", cfg.UI.Theme, defaults.UI.Theme)
	}
	if cfg.Defaults.CleanupThresholdDays != defaults.Defaults.CleanupThresholdDays {
		t.Errorf("CleanupThresholdDays = %d, want default %d",
			cfg.Defaults.CleanupThresholdDays, defaults.Defaults.CleanupThresholdDays)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "agm")
	os.MkdirAll(configDir, 0755)

	configContent := `
ui:
  theme: "agm-light"
  picker_height: 20
  no_color: true
defaults:
  interactive: false
  cleanup_threshold_days: 60
`
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644)

	cfg := LoadConfig()

	if cfg.UI.Theme != "agm-light" {
		t.Errorf("Theme = %q, want %q", cfg.UI.Theme, "agm-light")
	}
	if cfg.UI.PickerHeight != 20 {
		t.Errorf("PickerHeight = %d, want 20", cfg.UI.PickerHeight)
	}
	if !cfg.UI.NoColor {
		t.Error("NoColor should be true from config")
	}
	if cfg.Defaults.Interactive {
		t.Error("Interactive should be false from config")
	}
	if cfg.Defaults.CleanupThresholdDays != 60 {
		t.Errorf("CleanupThresholdDays = %d, want 60", cfg.Defaults.CleanupThresholdDays)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "agm")
	os.MkdirAll(configDir, 0755)

	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("invalid: [yaml: {{{"), 0644)

	cfg := LoadConfig()

	// Should return defaults on invalid YAML
	defaults := DefaultConfig()
	if cfg.UI.Theme != defaults.UI.Theme {
		t.Errorf("Theme = %q, want default %q on invalid YAML", cfg.UI.Theme, defaults.UI.Theme)
	}
}

func TestMergeWithDefaults(t *testing.T) {
	tests := []struct {
		name   string
		input  *Config
		check  func(*Config) bool
		errMsg string
	}{
		{
			name: "zero PickerHeight gets default",
			input: &Config{
				UI: UIConfig{PickerHeight: 0},
			},
			check:  func(c *Config) bool { return c.UI.PickerHeight == 15 },
			errMsg: "PickerHeight should be filled with default 15",
		},
		{
			name: "non-zero PickerHeight preserved",
			input: &Config{
				UI: UIConfig{PickerHeight: 25},
			},
			check:  func(c *Config) bool { return c.UI.PickerHeight == 25 },
			errMsg: "PickerHeight 25 should be preserved",
		},
		{
			name: "zero CleanupThresholdDays gets default",
			input: &Config{
				Defaults: DefaultsConfig{CleanupThresholdDays: 0},
			},
			check:  func(c *Config) bool { return c.Defaults.CleanupThresholdDays == 30 },
			errMsg: "CleanupThresholdDays should be filled with default 30",
		},
		{
			name: "zero ArchiveThresholdDays gets default",
			input: &Config{
				Defaults: DefaultsConfig{ArchiveThresholdDays: 0},
			},
			check:  func(c *Config) bool { return c.Defaults.ArchiveThresholdDays == 90 },
			errMsg: "ArchiveThresholdDays should be filled with default 90",
		},
		{
			name: "empty theme gets default",
			input: &Config{
				UI: UIConfig{Theme: ""},
			},
			check:  func(c *Config) bool { return c.UI.Theme == "agm" },
			errMsg: "Theme should be filled with default 'agm'",
		},
		{
			name: "custom theme preserved",
			input: &Config{
				UI: UIConfig{Theme: "dracula"},
			},
			check:  func(c *Config) bool { return c.UI.Theme == "dracula" },
			errMsg: "Theme 'dracula' should be preserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeWithDefaults(tt.input)
			if !tt.check(result) {
				t.Error(tt.errMsg)
			}
		})
	}
}

func TestSetGetGlobalConfig(t *testing.T) {
	// Reset to nil
	SetGlobalConfig(nil)
	cfg := GetGlobalConfig()
	if cfg == nil {
		t.Fatal("GetGlobalConfig should return default when nil")
	}

	// Set custom
	custom := &Config{
		UI: UIConfig{
			Theme:   "custom",
			NoColor: true,
		},
	}
	SetGlobalConfig(custom)

	got := GetGlobalConfig()
	if got.UI.Theme != "custom" {
		t.Errorf("Theme = %q, want %q", got.UI.Theme, "custom")
	}
	if !got.UI.NoColor {
		t.Error("NoColor should be true")
	}

	// Cleanup
	SetGlobalConfig(nil)
}
