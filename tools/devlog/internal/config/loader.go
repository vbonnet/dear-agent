// Package config provides configuration management.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	deverrors "github.com/vbonnet/dear-agent/tools/devlog/internal/errors"
)

const (
	// ConfigFileName is the name of the committed config file
	ConfigFileName = "config.yaml"
	// LocalConfigFileName is the name of the local (git-ignored) config file
	LocalConfigFileName = "config.local.yaml"
	// DefaultConfigDir is the default directory name for devlog configuration
	DefaultConfigDir = ".devlog"
)

// LoadMerged loads and merges committed and local config files.
// It searches for configs starting from the given directory and walking up.
func LoadMerged(searchPath string) (*Config, error) {
	configDir, err := findConfigDir(searchPath)
	if err != nil {
		return nil, err
	}

	baseConfigPath := filepath.Join(configDir, ConfigFileName)
	localConfigPath := filepath.Join(configDir, LocalConfigFileName)

	// Load base config (required)
	baseConfig, err := Load(baseConfigPath)
	if err != nil {
		return nil, err
	}

	// Load local config (optional)
	localConfig, err := Load(localConfigPath)
	if err != nil {
		// If local config doesn't exist, that's OK
		if !os.IsNotExist(err) && !errors.Is(err, deverrors.ErrConfigNotFound) {
			return nil, fmt.Errorf("failed to load local config: %w", err)
		}
		localConfig = nil
	}

	// Merge configs
	merged := Merge(baseConfig, localConfig)

	// Validate merged result
	if err := merged.Validate(); err != nil {
		return nil, err
	}

	return merged, nil
}

// findConfigDir searches for .devlog directory starting from current directory
// and walking up the directory tree.
func findConfigDir(startPath string) (string, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", deverrors.Wrap("resolve path", err)
	}

	current := absPath
	for {
		configDir := filepath.Join(current, DefaultConfigDir)
		configFile := filepath.Join(configDir, ConfigFileName)

		// Check if config file exists
		if _, err := os.Stat(configFile); err == nil {
			return configDir, nil
		}

		// Move up one directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}

	return "", deverrors.WrapPath("find config directory", startPath,
		fmt.Errorf("%w: searched from %s up to root", deverrors.ErrConfigNotFound, startPath))
}
