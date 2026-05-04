package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Hardcoded defaults used when no configuration file exists or
// when a specific tool/provider combination is not configured.
const (
	DefaultAnthropicModel = "claude-3-5-sonnet-20241022"
	DefaultGeminiModel    = "gemini-2.0-flash-exp"
)

// LoadConfig loads the LLM configuration from the specified path.
// If the file doesn't exist, it returns a config with hardcoded defaults.
// The path should typically be "~/.engram/llm-config.yaml".
func LoadConfig(path string) (*Config, error) {
	// Expand tilde to home directory
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// If file doesn't exist, return defaults
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return getDefaultConfig(), nil
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Ensure defaults are set
	if config.Defaults == nil {
		config.Defaults = &DefaultConfig{
			Anthropic: &ProviderConfig{Model: DefaultAnthropicModel},
			Gemini:    &ProviderConfig{Model: DefaultGeminiModel},
		}
	} else {
		if config.Defaults.Anthropic == nil {
			config.Defaults.Anthropic = &ProviderConfig{Model: DefaultAnthropicModel}
		} else if config.Defaults.Anthropic.Model == "" {
			config.Defaults.Anthropic.Model = DefaultAnthropicModel
		}
		if config.Defaults.Gemini == nil {
			config.Defaults.Gemini = &ProviderConfig{Model: DefaultGeminiModel}
		} else if config.Defaults.Gemini.Model == "" {
			config.Defaults.Gemini.Model = DefaultGeminiModel
		}
	}

	return &config, nil
}

// SelectModel returns the appropriate model for the given tool and provider family.
// It follows a three-tier fallback hierarchy:
//  1. Tool-specific provider configuration
//  2. Global defaults from config file
//  3. Hardcoded defaults
//
// Parameters:
//   - config: The loaded configuration (from LoadConfig)
//   - toolName: Name of the tool requesting a model (e.g., "ecphory", "multi-persona-review")
//   - providerFamily: Provider family to use (e.g., "anthropic", "gemini")
//
// Returns the model identifier string, or empty string if the provider is unknown.
//nolint:gocyclo // reason: linear model selection with many independent guards
func SelectModel(config *Config, toolName, providerFamily string) string {
	if config == nil {
		config = getDefaultConfig()
	}

	// 1. Check tool-specific configuration
	if config.Tools != nil {
		if toolConfig, ok := config.Tools[toolName]; ok && toolConfig != nil {
			var providerConfig *ProviderConfig
			switch providerFamily {
			case "anthropic", "claude":
				providerConfig = toolConfig.Anthropic
			case "gemini", "google":
				providerConfig = toolConfig.Gemini
			}

			if providerConfig != nil && providerConfig.Model != "" {
				return providerConfig.Model
			}
		}
	}

	// 2. Fall back to global defaults from config
	if config.Defaults != nil {
		var providerConfig *ProviderConfig
		switch providerFamily {
		case "anthropic", "claude":
			providerConfig = config.Defaults.Anthropic
		case "gemini", "google":
			providerConfig = config.Defaults.Gemini
		}

		if providerConfig != nil && providerConfig.Model != "" {
			return providerConfig.Model
		}
	}

	// 3. Hardcoded fallback
	switch providerFamily {
	case "anthropic", "claude":
		return DefaultAnthropicModel
	case "gemini", "google":
		return DefaultGeminiModel
	default:
		return ""
	}
}

// GetMaxTokens returns the max_tokens setting for a given tool and provider.
// Returns 0 if not configured (caller should use provider defaults).
func GetMaxTokens(config *Config, toolName, providerFamily string) int {
	if config == nil {
		return 0
	}

	// Check tool-specific configuration
	if config.Tools != nil {
		if toolConfig, ok := config.Tools[toolName]; ok && toolConfig != nil {
			var providerConfig *ProviderConfig
			switch providerFamily {
			case "anthropic", "claude":
				providerConfig = toolConfig.Anthropic
			case "gemini", "google":
				providerConfig = toolConfig.Gemini
			}

			if providerConfig != nil && providerConfig.MaxTokens > 0 {
				return providerConfig.MaxTokens
			}
		}
	}

	// Fall back to global defaults
	if config.Defaults != nil {
		var providerConfig *ProviderConfig
		switch providerFamily {
		case "anthropic", "claude":
			providerConfig = config.Defaults.Anthropic
		case "gemini", "google":
			providerConfig = config.Defaults.Gemini
		}

		if providerConfig != nil && providerConfig.MaxTokens > 0 {
			return providerConfig.MaxTokens
		}
	}

	return 0
}

// getDefaultConfig returns a config with hardcoded defaults.
func getDefaultConfig() *Config {
	return &Config{
		Defaults: &DefaultConfig{
			Anthropic: &ProviderConfig{
				Model: DefaultAnthropicModel,
			},
			Gemini: &ProviderConfig{
				Model: DefaultGeminiModel,
			},
		},
	}
}
