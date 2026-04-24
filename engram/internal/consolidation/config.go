package consolidation

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config defines provider configuration.
//
// Configuration can be loaded from:
//  1. .engram/config.yaml in current/parent directories (project-specific)
//  2. ~/.config/engram/config.yaml (global)
//  3. Built-in defaults
//
// Example config.yaml:
//
//	memory:
//	  provider: simple
//	  simple:
//	    storage_path: ~/.engram/memory
type Config struct {
	// ProviderType identifies the provider implementation ("simple", etc.)
	ProviderType string `yaml:"provider_type"`

	// Options contains provider-specific configuration.
	// SimpleFileProvider expects: {"storage_path": "/path/to/storage"}
	Options map[string]interface{} `yaml:"options"`
}

// LoadConfig discovers and loads configuration.
//
// Discovery order:
//  1. .engram/config.yaml in current/parent directories (walk up to root)
//  2. ~/.config/engram/config.yaml (global)
//  3. Built-in defaults (SimpleFileProvider at ~/.engram/memory)
//
// Project config overrides global config when both exist.
//
// Example:
//
//	config, err := consolidation.LoadConfig()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	provider, err := consolidation.Load(config)
func LoadConfig() (Config, error) {
	// 1. Check for project-specific config
	if cfg, err := loadProjectConfig(); err == nil {
		return cfg, nil
	}

	// 2. Check for global config
	if cfg, err := loadGlobalConfig(); err == nil {
		return cfg, nil
	}

	// 3. Return defaults
	return getDefaultConfig(), nil
}

// loadProjectConfig walks up from current directory looking for .engram/config.yaml
func loadProjectConfig() (Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	for {
		configPath := filepath.Join(dir, ".engram", "config.yaml")
		if cfg, err := loadConfigFile(configPath); err == nil {
			return cfg, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}

	return Config{}, os.ErrNotExist
}

// loadGlobalConfig loads config from ~/.config/engram/config.yaml
func loadGlobalConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}

	configPath := filepath.Join(home, ".config", "engram", "config.yaml")
	return loadConfigFile(configPath)
}

// loadConfigFile reads and parses a YAML config file
func loadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// Support nested structure (memory.provider, memory.simple.storage_path)
	type MemoryConfig struct {
		Provider string                            `yaml:"provider"`
		Simple   map[string]interface{}            `yaml:"simple"`
		Options  map[string]map[string]interface{} `yaml:",inline"`
	}

	type RootConfig struct {
		Memory MemoryConfig `yaml:"memory"`
	}

	var root RootConfig
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Config{}, err
	}

	// Extract provider type and options
	providerType := root.Memory.Provider
	if providerType == "" {
		providerType = "simple" // Default
	}

	// Build options map from provider-specific config
	options := make(map[string]interface{})
	if providerType == "simple" && root.Memory.Simple != nil {
		for k, v := range root.Memory.Simple {
			options[k] = v
		}
	}

	return Config{
		ProviderType: providerType,
		Options:      options,
	}, nil
}

// getDefaultConfig returns built-in defaults
func getDefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": filepath.Join(home, ".engram", "memory"),
		},
	}
}
