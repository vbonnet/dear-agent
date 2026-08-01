package earslint

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig is the root-module YAML adapter for the canonical, stdlib-only
// EARS core projected into this package. Installed SPEC-governance tooling does
// not compile this adapter or depend on a module cache.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
	}
	defaults := DefaultConfig()
	if cfg.RequirementKeyword == "" {
		cfg.RequirementKeyword = defaults.RequirementKeyword
	}
	if len(cfg.Patterns) == 0 {
		cfg.Patterns = defaults.Patterns
	}
	return cfg, nil
}
