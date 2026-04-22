package ui

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// globalConfig holds the active UI configuration
var globalConfig *Config

// SetGlobalConfig sets the global UI configuration (called from main package)
func SetGlobalConfig(cfg *Config) {
	globalConfig = cfg
}

// GetGlobalConfig returns the global UI configuration
func GetGlobalConfig() *Config {
	if globalConfig == nil {
		return DefaultConfig()
	}
	return globalConfig
}

// Config represents UI configuration
type Config struct {
	Defaults DefaultsConfig `yaml:"defaults"`
	UI       UIConfig       `yaml:"ui"`
	Advanced AdvancedConfig `yaml:"advanced"`
}

type DefaultsConfig struct {
	Interactive          bool `yaml:"interactive"`
	AutoAssociateUUID    bool `yaml:"auto_associate_uuid"`
	ConfirmDestructive   bool `yaml:"confirm_destructive"`
	CleanupThresholdDays int  `yaml:"cleanup_threshold_days"`
	ArchiveThresholdDays int  `yaml:"archive_threshold_days"`
}

type UIConfig struct {
	Theme            string `yaml:"theme"`
	PickerHeight     int    `yaml:"picker_height"`
	ShowProjectPaths bool   `yaml:"show_project_paths"`
	ShowTags         bool   `yaml:"show_tags"`
	FuzzySearch      bool   `yaml:"fuzzy_search"`
	NoColor          bool   `yaml:"no_color"`      // Disable colored output (WCAG AA)
	ScreenReader     bool   `yaml:"screen_reader"` // Use text symbols instead of Unicode
}

type AdvancedConfig struct {
	TmuxTimeout         string `yaml:"tmux_timeout"`
	HealthCheckCache    string `yaml:"health_check_cache"`
	LockTimeout         string `yaml:"lock_timeout"`
	UUIDDetectionWindow string `yaml:"uuid_detection_window"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Defaults: DefaultsConfig{
			Interactive:          true,
			AutoAssociateUUID:    true,
			ConfirmDestructive:   true,
			CleanupThresholdDays: 30,
			ArchiveThresholdDays: 90,
		},
		UI: UIConfig{
			Theme:            "agm", // High-contrast custom theme (changed from "dracula")
			PickerHeight:     15,
			ShowProjectPaths: true,
			ShowTags:         true,
			FuzzySearch:      true,
		},
		Advanced: AdvancedConfig{
			TmuxTimeout:         "5s",
			HealthCheckCache:    "5s",
			LockTimeout:         "30s",
			UUIDDetectionWindow: "5m",
		},
	}
}

// LoadConfig loads configuration from ~/.config/agm/config.yaml
// Returns default config if file doesn't exist or is invalid
func LoadConfig() *Config {
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "agm", "config.yaml")

	// Use defaults if file doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Warn but continue with defaults
		return DefaultConfig()
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// Invalid YAML, use defaults
		return DefaultConfig()
	}

	// Merge with defaults for missing fields
	return mergeWithDefaults(&cfg)
}

func mergeWithDefaults(cfg *Config) *Config {
	defaults := DefaultConfig()

	// If values are zero, use defaults
	if cfg.UI.PickerHeight == 0 {
		cfg.UI.PickerHeight = defaults.UI.PickerHeight
	}
	if cfg.Defaults.CleanupThresholdDays == 0 {
		cfg.Defaults.CleanupThresholdDays = defaults.Defaults.CleanupThresholdDays
	}
	if cfg.Defaults.ArchiveThresholdDays == 0 {
		cfg.Defaults.ArchiveThresholdDays = defaults.Defaults.ArchiveThresholdDays
	}
	if cfg.UI.Theme == "" {
		cfg.UI.Theme = defaults.UI.Theme
	}

	return cfg
}
