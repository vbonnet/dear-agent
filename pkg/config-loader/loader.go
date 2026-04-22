package configloader

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads a YAML config file and unmarshals it into the provided type.
// Supports home directory expansion (~/) in the path.
//
// Returns an error if:
//   - The file cannot be read (permission denied, not found, etc.)
//   - The YAML is invalid or cannot be unmarshaled into type T
//
// Example:
//
//	type MyConfig struct {
//	    Name string `yaml:"name"`
//	}
//	cfg, err := configloader.Load[MyConfig]("~/.config/app/config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
func Load[T any](path string) (*T, error) {
	// Expand home directory
	expandedPath, err := ExpandHome(path)
	if err != nil {
		return nil, fmt.Errorf("expand path %q: %w", path, err)
	}

	// Read file
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", expandedPath, err)
	}

	// Unmarshal YAML
	var cfg T
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal YAML from %q: %w", expandedPath, err)
	}

	return &cfg, nil
}

// LoadWithDefaults loads config from file, falling back to defaults if file is missing.
// Still returns an error if the file exists but contains invalid YAML.
//
// This is useful for optional config files where you want to use defaults
// when the file doesn't exist, but still want to catch configuration errors.
//
// Example:
//
//	defaults := MyConfig{Name: "default", Timeout: 30}
//	cfg, err := configloader.LoadWithDefaults("config.yaml", defaults)
//	if err != nil {
//	    // File exists but has invalid YAML
//	    log.Fatal(err)
//	}
//	// cfg is either loaded from file or defaults
func LoadWithDefaults[T any](path string, defaults T) (*T, error) {
	cfg, err := Load[T](path)
	if err != nil {
		// If file doesn't exist, use defaults (not an error)
		// Use errors.Is to properly check wrapped errors
		if errors.Is(err, os.ErrNotExist) {
			return &defaults, nil
		}
		// Other errors (permission denied, invalid YAML) still propagate
		return nil, err
	}
	return cfg, nil
}

// LoadOrDefault loads config from file, always returning valid config.
// Never returns an error - uses defaults on any failure (missing file, invalid YAML, etc.).
//
// This is useful when you want configuration to be truly optional and
// any config errors should be silently ignored.
//
// Example:
//
//	defaults := MyConfig{Name: "default"}
//	cfg := configloader.LoadOrDefault("config.yaml", defaults)
//	// cfg is guaranteed to be non-nil
func LoadOrDefault[T any](path string, defaults T) *T {
	cfg, err := LoadWithDefaults(path, defaults)
	if err != nil {
		// On any error (invalid YAML, permission denied, etc.), use defaults
		return &defaults
	}
	return cfg
}
