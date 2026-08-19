package ui

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
		return
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
