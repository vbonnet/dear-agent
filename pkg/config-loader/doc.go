// Package configloader provides generic YAML configuration loading with
// home directory expansion and default fallback support.
//
// This package consolidates common configuration loading patterns found across
// engram and ai-tools repositories, eliminating ~150 LOC of duplicated code.
//
// Features:
//   - Generic loader supporting any config struct type via Go generics
//   - Automatic home directory (~/) expansion in file paths
//   - Graceful fallback to defaults for optional configuration files
//   - Comprehensive error messages with context
//   - Zero dependencies beyond gopkg.in/yaml.v3
//
// # Basic Usage
//
// Define your configuration struct:
//
//	type MyConfig struct {
//	    Name    string `yaml:"name"`
//	    Timeout int    `yaml:"timeout"`
//	}
//
// Load required configuration:
//
//	cfg, err := configloader.Load[MyConfig]("~/.config/app/config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Load optional configuration with defaults:
//
//	defaults := MyConfig{Name: "default", Timeout: 30}
//	cfg := configloader.LoadOrDefault("config.yaml", defaults)
//
// # Home Directory Expansion
//
// All loading functions automatically expand ~ to the user's home directory:
//
//	"~/.config/app/config.yaml"    → "~/.config/app/config.yaml"
//	"~/config.yaml"                → "~/config.yaml"
//	"/etc/app/config.yaml"         → "/etc/app/config.yaml" (unchanged)
//
// # Error Handling
//
// The package provides three loading functions with different error handling:
//
//   - Load: Returns error on any failure (file not found, invalid YAML, etc.)
//   - LoadWithDefaults: Returns defaults if file missing, error if invalid YAML
//   - LoadOrDefault: Never returns error, always returns valid config
//
// Choose based on whether configuration is required, optional, or truly optional:
//
//	// Required config - fail if missing or invalid
//	cfg, err := configloader.Load[MyConfig]("config.yaml")
//
//	// Optional config - use defaults if missing, fail if invalid
//	cfg, err := configloader.LoadWithDefaults("config.yaml", defaults)
//
//	// Truly optional - never fail, silently use defaults
//	cfg := configloader.LoadOrDefault("config.yaml", defaults)
//
// # Validation
//
// This library handles YAML parsing and path expansion only. Consumers
// are responsible for validating the semantic correctness of loaded configuration:
//
//	cfg, err := configloader.Load[MyConfig]("config.yaml")
//	if err != nil {
//	    return err
//	}
//
//	// Validate business logic constraints
//	if err := cfg.Validate(); err != nil {
//	    return fmt.Errorf("invalid config: %w", err)
//	}
//
// # Environment Variable Overrides
//
// This library does not handle environment variable overrides, as each
// consumer has different naming conventions (CSM_*, SWARM_*, AGM_ENGRAM_*).
// Apply environment overrides after loading:
//
//	cfg, _ := configloader.Load[MyConfig]("config.yaml")
//	if dir := os.Getenv("APP_DATA_DIR"); dir != "" {
//	    cfg.DataDir = dir
//	}
//
// See individual function documentation for detailed usage and examples.
package configloader
