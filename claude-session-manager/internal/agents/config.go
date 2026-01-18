package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AgentsConfig represents AGENTS.md configuration for agent routing
type AgentsConfig struct {
	SchemaVersion string       `yaml:"schema_version"` // Optional: for future migration
	DefaultAgent  string       `yaml:"default_agent"`  // Required: fallback agent
	Preferences   []Preference `yaml:"preferences"`    // Optional: keyword-based routing rules
}

// Preference represents a keyword-based agent routing rule
type Preference struct {
	Keywords []string `yaml:"keywords"` // Required: session name substrings to match
	Agent    string   `yaml:"agent"`    // Required: agent to use when keywords match
}

// DefaultConfig returns the system default configuration
func DefaultConfig() *AgentsConfig {
	return &AgentsConfig{
		SchemaVersion: "1.0",
		DefaultAgent:  "claude",
		Preferences:   []Preference{},
	}
}

// LoadConfig loads AGENTS.md configuration from multi-path detection.
// Returns default config if no file found (graceful fallback).
// Detection order (first found wins):
//  1. ./AGENTS.md (local project)
//  2. ~/.config/agm/AGENTS.md (global config)
//  3. No file found → DefaultConfig()
func LoadConfig() *AgentsConfig {
	// Multi-path detection
	homeDir, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(".", "AGENTS.md"),
		filepath.Join(homeDir, ".config", "agm", "AGENTS.md"),
	}

	for _, path := range paths {
		if config, ok := tryLoad(path); ok {
			return config // First valid config found
		}
	}

	// No config file found → use defaults (not an error)
	return DefaultConfig()
}

// tryLoad attempts to load config from a single path.
// Returns (config, true) if successful, (nil, false) if file not found.
func tryLoad(path string) (*AgentsConfig, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false // File not found or permission denied
	}

	var config AgentsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		// YAML parse error → warn user + fallback to defaults
		fmt.Fprintf(os.Stderr, "Warning: %s parse error: %v\n", path, err)
		fmt.Fprintf(os.Stderr, "Using default agent configuration.\n")
		return DefaultConfig(), true // Use defaults but stop searching
	}

	// Validate required fields
	if config.DefaultAgent == "" {
		fmt.Fprintf(os.Stderr, "Warning: %s missing default_agent field\n", path)
		config.DefaultAgent = "claude" // Use system default
	}

	// Validate preferences (filter out invalid ones)
	validPrefs := []Preference{}
	for i, pref := range config.Preferences {
		if len(pref.Keywords) == 0 {
			fmt.Fprintf(os.Stderr, "Warning: %s preference #%d has empty keywords list, skipping\n", path, i+1)
			continue // Skip invalid preference
		}
		if pref.Agent == "" {
			fmt.Fprintf(os.Stderr, "Warning: %s preference #%d has empty agent field, skipping\n", path, i+1)
			continue
		}
		validPrefs = append(validPrefs, pref)
	}
	config.Preferences = validPrefs

	// Set default schema version if missing
	if config.SchemaVersion == "" {
		config.SchemaVersion = "1.0"
	}

	return &config, true // Valid config loaded
}
