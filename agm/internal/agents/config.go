// Package agents provides agents functionality.
package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// HarnessConfig represents AGENTS.md configuration for harness routing
type HarnessConfig struct {
	SchemaVersion  string       `yaml:"schema_version"`  // Optional: for future migration
	DefaultHarness string       `yaml:"default_harness"` // Required: fallback harness
	Preferences    []Preference `yaml:"preferences"`     // Optional: keyword-based routing rules
}

// Preference represents a keyword-based harness routing rule
type Preference struct {
	Keywords []string `yaml:"keywords"` // Required: session name substrings to match
	Harness  string   `yaml:"harness"`  // Required: harness to use when keywords match
}

// DefaultConfig returns the system default configuration
func DefaultConfig() *HarnessConfig {
	return &HarnessConfig{
		SchemaVersion:  "1.0",
		DefaultHarness: "claude-code",
		Preferences:    []Preference{},
	}
}

// LoadConfig loads AGENTS.md configuration from multi-path detection.
// Returns default config if no file found (graceful fallback).
// Detection order (first found wins):
//  1. ./AGENTS.md (local project)
//  2. ~/.config/agm/AGENTS.md (global config)
//  3. No file found → DefaultConfig()
func LoadConfig() *HarnessConfig {
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
func tryLoad(path string) (*HarnessConfig, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false // File not found or permission denied
	}

	var config HarnessConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		// YAML parse error → warn user + fallback to defaults
		fmt.Fprintf(os.Stderr, "Warning: %s parse error: %v\n", path, err)
		fmt.Fprintf(os.Stderr, "Using default harness configuration.\n")
		return DefaultConfig(), true // Use defaults but stop searching
	}

	// Validate required fields
	if config.DefaultHarness == "" {
		fmt.Fprintf(os.Stderr, "Warning: %s missing default_harness field\n", path)
		config.DefaultHarness = "claude-code" // Use system default
	}

	// Validate preferences (filter out invalid ones)
	validPrefs := []Preference{}
	for i, pref := range config.Preferences {
		if len(pref.Keywords) == 0 {
			fmt.Fprintf(os.Stderr, "Warning: %s preference #%d has empty keywords list, skipping\n", path, i+1)
			continue // Skip invalid preference
		}
		if pref.Harness == "" {
			fmt.Fprintf(os.Stderr, "Warning: %s preference #%d has empty harness field, skipping\n", path, i+1)
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
