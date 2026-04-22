package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads and validates workspace config from YAML file.
func LoadConfig(path string) (*Config, error) {
	// Expand path
	expandedPath := ExpandHome(path)

	// Check if file exists
	if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, expandedPath)
	}

	// Read file
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate config
	if err := ValidateConfig(&config); err != nil {
		return nil, err
	}

	// Expand paths in config
	if err := ExpandPaths(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig writes config to YAML file.
func SaveConfig(path string, config *Config) error {
	// Validate before saving
	if err := ValidateConfig(config); err != nil {
		return err
	}

	// Expand path
	expandedPath := ExpandHome(path)

	// Ensure directory exists
	dir := filepath.Dir(expandedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Write file with user-only permissions
	if err := os.WriteFile(expandedPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfig checks config for errors.
func ValidateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("%w: config is nil", ErrInvalidConfig)
	}

	// Check version
	if config.Version != 1 {
		return fmt.Errorf("%w: expected version 1, got %d", ErrInvalidVersion, config.Version)
	}

	// Check workspaces exist
	if len(config.Workspaces) == 0 {
		return fmt.Errorf("%w: no workspaces defined", ErrInvalidConfig)
	}

	// Check at least one enabled workspace
	hasEnabled := false
	workspaceNames := make(map[string]bool)

	for i, ws := range config.Workspaces {
		// Check required fields
		if ws.Name == "" {
			return fmt.Errorf("%w: workspace %d has empty name", ErrInvalidConfig, i)
		}
		if ws.Root == "" {
			return fmt.Errorf("%w: workspace '%s' has empty root", ErrInvalidConfig, ws.Name)
		}

		// Check for duplicate names
		if workspaceNames[ws.Name] {
			return fmt.Errorf("%w: duplicate workspace name '%s'", ErrInvalidConfig, ws.Name)
		}
		workspaceNames[ws.Name] = true

		// Check if enabled
		if ws.Enabled {
			hasEnabled = true
		}
	}

	if !hasEnabled {
		return fmt.Errorf("%w", ErrNoEnabledWorkspaces)
	}

	// Check default workspace exists (if specified)
	if config.DefaultWorkspace != "" {
		if !workspaceNames[config.DefaultWorkspace] {
			return fmt.Errorf("%w: default workspace '%s' not found in workspaces list",
				ErrInvalidConfig, config.DefaultWorkspace)
		}
	}

	return nil
}

// ExpandPaths expands ~ and environment variables in all paths.
func ExpandPaths(config *Config) error {
	for i := range config.Workspaces {
		ws := &config.Workspaces[i]

		// Expand root path
		expandedRoot, err := NormalizePath(ws.Root)
		if err != nil {
			return fmt.Errorf("invalid root path for workspace '%s': %w", ws.Name, err)
		}
		ws.Root = expandedRoot

		// Validate root is absolute
		if err := ValidateAbsolutePath(ws.Root); err != nil {
			return fmt.Errorf("workspace '%s' root: %w", ws.Name, err)
		}

		// Expand output_dir if specified
		if ws.OutputDir != "" {
			expandedOutput, err := NormalizePath(ws.OutputDir)
			if err != nil {
				return fmt.Errorf("invalid output_dir for workspace '%s': %w", ws.Name, err)
			}
			ws.OutputDir = expandedOutput
		} else {
			// Default to {root}/.{tool} - but we don't know tool name here
			// Tools should set this themselves if not specified
			ws.OutputDir = ws.Root
		}
	}

	return nil
}

// GetDefaultConfigPath returns platform-specific default config location.
func GetDefaultConfigPath(toolName string) string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}
	return filepath.Join(home, fmt.Sprintf(".%s", toolName), "config.yaml")
}

// GenerateDefaultConfig creates a default config with common workspace setups.
func GenerateDefaultConfig() *Config {
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}

	return &Config{
		Version:          1,
		DefaultWorkspace: "default",
		Workspaces: []Workspace{
			{
				Name:      "default",
				Root:      filepath.Join(home, "src"),
				OutputDir: filepath.Join(home, "src"),
				Enabled:   true,
			},
		},
	}
}
