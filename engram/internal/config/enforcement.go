package config

// EnforcementConfig configures company policy enforcement.
//
// Enforcement validates that users comply with company requirements:
//   - Identity verification (allowed email domains)
//   - Required plugins installed
//   - Required config values set
//
// Only Core and Company tiers can enable enforcement. This prevents users
// from bypassing company requirements via local configuration.
//
// See core/docs/adr/enforcement-architecture.md for design details.
type EnforcementConfig struct {
	// Enable enforcement validation
	Enabled bool `yaml:"enabled"`

	// Fail mode: "hard" (exit on violation) or "soft" (warn only)
	FailMode string `yaml:"fail_mode"`

	// Identity verification configuration
	Identity EnforcementIdentityConfig `yaml:"identity"`

	// Required plugins (with optional version constraints)
	Plugins []PluginRequirement `yaml:"required_plugins"`

	// Required config values
	Config []ConfigRequirement `yaml:"required_config"`

	// Custom validator plugin references
	Validators []CustomValidator `yaml:"custom_validators"`

	// Error message customization
	ErrorMessages *ErrorMessageConfig `yaml:"error_messages,omitempty"`
}

// EnforcementIdentityConfig configures identity detection and verification.
type EnforcementIdentityConfig struct {
	// Require identity verification
	Required bool `yaml:"required"`

	// Detection methods to try (e.g., "gcp_adc", "git_config", "env")
	// Order determines priority. If empty, all methods are tried.
	Methods []string `yaml:"methods"`

	// Allowed email domains (e.g., "@acme.com", "@company.com")
	// If empty, any domain is allowed
	AllowedDomains []string `yaml:"allowed_domains"`

	// Cache TTL in hours (default: 24)
	CacheTTLHours int `yaml:"cache_ttl_hours"`

	// Fail mode: "hard" (exit on verification failure) or "soft" (warn only)
	FailMode string `yaml:"fail_mode"`
}

// PluginRequirement specifies a required plugin with version constraints.
type PluginRequirement struct {
	// Plugin name (e.g., "acme-integration")
	Name string `yaml:"name"`

	// Minimum version (semver, e.g., "1.0.0")
	VersionMin string `yaml:"version_min"`

	// Maximum version (optional, semver)
	VersionMax *string `yaml:"version_max,omitempty"`

	// Source location (optional, for installation hints)
	Source string `yaml:"source,omitempty"`
}

// ConfigRequirement specifies a required configuration value.
type ConfigRequirement struct {
	// Config key (dot notation, e.g., "telemetry.enabled")
	Key string `yaml:"key"`

	// Required value
	Value interface{} `yaml:"value"`

	// Tier that enforces this requirement ("core" or "company")
	// Prevents users from overriding company-required config
	EnforceTier string `yaml:"enforce_tier"`
}

// CustomValidator references a plugin that provides custom validation logic.
type CustomValidator struct {
	// Plugin name that implements custom validation
	Plugin string `yaml:"plugin"`

	// Enable this validator
	Enabled bool `yaml:"enabled"`
}

// ErrorMessageConfig customizes enforcement error messages.
type ErrorMessageConfig struct {
	// Go template for error messages
	// Available fields: .Message (string), .Actions ([]string), .HelpURL (string)
	Template string `yaml:"template"`

	// Help URL for enforcement documentation
	HelpURL string `yaml:"help_url"`
}
