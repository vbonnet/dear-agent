// Package config provides configuration management for LLM provider preferences.
// It supports per-tool model selection with fallback to global defaults.
package config

// Config represents the complete LLM configuration with per-tool preferences
// and global defaults. It's loaded from ~/.engram/llm-config.yaml.
type Config struct {
	// Tools maps tool names to their provider-specific configurations.
	// Example: "ecphory" -> {anthropic: {...}, gemini: {...}}
	Tools map[string]*ToolConfig `yaml:"tools,omitempty"`

	// Defaults provides global fallback settings when a tool-specific
	// configuration is not available.
	Defaults *DefaultConfig `yaml:"defaults,omitempty"`
}

// ToolConfig holds provider-specific settings for a single tool.
// It allows different tools to use different models based on their
// cost/accuracy requirements.
type ToolConfig struct {
	// Anthropic provider configuration (Claude models)
	Anthropic *ProviderConfig `yaml:"anthropic,omitempty"`

	// Gemini provider configuration (Google models)
	Gemini *ProviderConfig `yaml:"gemini,omitempty"`

	// DefaultFamily specifies which provider to prefer when no explicit
	// provider is specified. Valid values: "anthropic", "gemini"
	DefaultFamily string `yaml:"default_family,omitempty"`
}

// ProviderConfig specifies model and token limits for a provider.
type ProviderConfig struct {
	// Model is the specific model identifier to use.
	// Examples: "claude-3-5-sonnet-20241022", "gemini-2.0-flash-exp"
	Model string `yaml:"model"`

	// MaxTokens is the maximum number of tokens to generate.
	// Optional; if not set, provider defaults will be used.
	MaxTokens int `yaml:"max_tokens,omitempty"`
}

// DefaultConfig provides global fallback settings for each provider family.
type DefaultConfig struct {
	// Anthropic default configuration
	Anthropic *ProviderConfig `yaml:"anthropic,omitempty"`

	// Gemini default configuration
	Gemini *ProviderConfig `yaml:"gemini,omitempty"`
}
