package ui

import "github.com/vbonnet/dear-agent/agm/internal/config"

// globalConfig holds the active UI projection from the validated shared
// configuration snapshot.
var globalConfig *Config

// SetGlobalConfig sets the active UI configuration.
func SetGlobalConfig(cfg *Config) {
	globalConfig = cfg
}

// GetGlobalConfig returns the active projection, or defaults before root
// command initialization has installed a snapshot.
func GetGlobalConfig() *Config {
	if globalConfig == nil {
		return DefaultConfig()
	}
	return globalConfig
}

// Aliases preserve the UI package's consumer-facing types while config owns
// the complete schema for the one shared YAML document.
type Config = config.UISettings
type DefaultsConfig = config.DefaultsConfig
type UIConfig = config.UIConfig
type AdvancedConfig = config.AdvancedConfig

// DefaultConfig returns the shared UI defaults.
func DefaultConfig() *Config {
	settings := config.DefaultUISettings()
	return &settings
}
