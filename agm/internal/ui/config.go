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

// Config aliases the shared UI settings owned by the config package.
type Config = config.UISettings

// DefaultsConfig aliases the shared default-behavior settings.
type DefaultsConfig = config.DefaultsConfig

// UIConfig aliases the shared presentation settings.
type UIConfig = config.UIConfig

// DefaultConfig returns the shared UI defaults.
func DefaultConfig() *Config {
	settings := config.DefaultUISettings()
	return &settings
}
