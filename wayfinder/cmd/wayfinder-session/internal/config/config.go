// Package config provides config-related functionality.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the Wayfinder configuration
type Config struct {
	Storage StorageConfig `yaml:"storage"`
}

// StorageConfig defines storage mode and location settings
type StorageConfig struct {
	Mode            string `yaml:"mode"`             // "dotfile" or "centralized"
	Workspace       string `yaml:"workspace"`        // workspace name or path
	RelativePath    string `yaml:"relative_path"`    // path within workspace
	CentralizedPath string `yaml:"centralized_path"` // explicit path override
	AutoSymlink     bool   `yaml:"auto_symlink"`     // auto-create symlinks
}

const (
	// Storage modes
	ModeDotfile     = "dotfile"
	ModeCentralized = "centralized"

	// Default paths
	DefaultDotfilePath  = "~/.wayfinder"
	DefaultRelativePath = "wf" // Changed from .wayfinder to wf (Wayfinder uses wf/ not .wayfinder/)
	DefaultWorkspace    = "engram-research"
)

// Load loads configuration from ~/.wayfinder/config.yaml
// Returns default config if file doesn't exist
func Load() (*Config, error) {
	configPath := getConfigPath()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default config (centralized mode - Wayfinder's current behavior)
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse YAML
	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("invalid config YAML: %w", err)
	}

	// Apply defaults for missing fields
	if config.Storage.Mode == "" {
		config.Storage.Mode = ModeCentralized
	}
	if config.Storage.RelativePath == "" {
		config.Storage.RelativePath = DefaultRelativePath
	}
	if config.Storage.Workspace == "" && config.Storage.Mode == ModeCentralized {
		config.Storage.Workspace = DefaultWorkspace
	}

	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Save saves configuration to ~/.wayfinder/config.yaml
func (c *Config) Save() error {
	configPath := getConfigPath()

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate mode
	if c.Storage.Mode != ModeDotfile && c.Storage.Mode != ModeCentralized {
		return fmt.Errorf("invalid storage.mode: must be '%s' or '%s', got '%s'",
			ModeDotfile, ModeCentralized, c.Storage.Mode)
	}

	// Centralized mode requirements
	if c.Storage.Mode == ModeCentralized {
		if c.Storage.Workspace == "" && c.Storage.CentralizedPath == "" {
			return fmt.Errorf("storage.workspace required when mode=centralized (or set centralized_path)")
		}
	}

	// Relative path safety
	if strings.Contains(c.Storage.RelativePath, "../") {
		return fmt.Errorf("storage.relative_path cannot contain '../'")
	}
	if filepath.IsAbs(c.Storage.RelativePath) {
		return fmt.Errorf("storage.relative_path must be relative, not absolute")
	}

	// Centralized path validation
	if c.Storage.CentralizedPath != "" {
		expanded := expandPath(c.Storage.CentralizedPath)
		if !filepath.IsAbs(expanded) {
			return fmt.Errorf("storage.centralized_path must be absolute")
		}
	}

	return nil
}

// GetStoragePath returns the absolute path where Wayfinder data should be stored
func (c *Config) GetStoragePath() (string, error) {
	if c.Storage.Mode == ModeDotfile {
		// Dotfile mode: use ~/.wayfinder/
		return expandPath(DefaultDotfilePath), nil
	}

	// Centralized mode: detect workspace and construct path
	// Priority 1: Explicit centralized_path
	if c.Storage.CentralizedPath != "" {
		return expandPath(c.Storage.CentralizedPath), nil
	}

	// Priority 2: Workspace detection
	workspace, err := DetectWorkspace(c.Storage.Workspace)
	if err != nil {
		return "", fmt.Errorf("failed to detect workspace: %w", err)
	}

	// Construct path: workspace/wf/
	storagePath := filepath.Join(workspace, c.Storage.RelativePath)
	return storagePath, nil
}

// DetectWorkspace implements workspace detection algorithm
// Returns absolute path to workspace directory
func DetectWorkspace(nameOrPath string) (string, error) {
	// Priority 1: Explicit absolute path
	if filepath.IsAbs(nameOrPath) {
		if _, err := os.Stat(nameOrPath); err == nil {
			return nameOrPath, nil
		}
		return "", fmt.Errorf("workspace path does not exist: %s", nameOrPath)
	}

	// Priority 2: Test mode
	if os.Getenv("ENGRAM_TEST_MODE") == "1" {
		testWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")
		if testWorkspace != "" {
			return testWorkspace, nil
		}
	}

	// Priority 3: Environment variable override
	if envWorkspace := os.Getenv("ENGRAM_WORKSPACE"); envWorkspace != "" {
		return envWorkspace, nil
	}

	// Priority 4: Auto-detect from current directory
	cwd, err := os.Getwd()
	if err == nil {
		if workspace := searchUpwardForWorkspace(cwd, nameOrPath); workspace != "" {
			return workspace, nil
		}
	}

	// Priority 5: Search common locations
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	commonLocations := []string{
		filepath.Join(home, "src", "ws", "oss", "repos", nameOrPath),
		filepath.Join(home, "src", nameOrPath),
		filepath.Join(home, nameOrPath),
	}

	for _, loc := range commonLocations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	// Not found
	return "", fmt.Errorf("workspace '%s' not found (searched common locations)", nameOrPath)
}

// searchUpwardForWorkspace searches parent directories for workspace markers
func searchUpwardForWorkspace(startDir, targetName string) string {
	dir := startDir
	for {
		// Check if this directory matches the workspace name and has markers
		if filepath.Base(dir) == targetName {
			if hasWorkspaceMarker(dir) {
				return dir
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return ""
}

// hasWorkspaceMarker checks if directory has workspace identification markers
func hasWorkspaceMarker(dir string) bool {
	// Check for .git directory
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}

	// Check for WORKSPACE.yaml
	if _, err := os.Stat(filepath.Join(dir, "WORKSPACE.yaml")); err == nil {
		return true
	}

	return false
}

// expandPath expands ~/ and environment variables in path
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	return filepath.Join(expandPath(DefaultDotfilePath), "config.yaml")
}

// DefaultConfig returns the default configuration
// Defaults to centralized mode (Wayfinder's current behavior)
func DefaultConfig() *Config {
	return &Config{
		Storage: StorageConfig{
			Mode:         ModeCentralized, // Default: centralized (current behavior)
			Workspace:    DefaultWorkspace,
			RelativePath: DefaultRelativePath,
			AutoSymlink:  true,
		},
	}
}
