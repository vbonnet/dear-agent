// Package phasegraph provides phase dependency graph configuration
// with full/summary/skip loading strategies and V1-to-V2 phase name mapping.
package phasegraph

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadStrategy specifies how a dependency artifact should be loaded.
type LoadStrategy string

const (
	// Full loads the complete artifact content.
	Full LoadStrategy = "full"
	// Summary loads a 100-200 token summary of the artifact.
	Summary LoadStrategy = "summary"
	// Skip means the dependency is not loaded at all.
	Skip LoadStrategy = "skip"
)

// PhaseDependencyConfig holds the parsed YAML configuration for
// phase dependencies and V1-to-V2 name mapping.
type PhaseDependencyConfig struct {
	Dependencies map[string]map[string]LoadStrategy `yaml:"dependencies"`
	V1ToV2       map[string]string                  `yaml:"v1_to_v2"`
}

// LoadConfig reads and parses a phase dependency YAML config file.
func LoadConfig(configPath string) (*PhaseDependencyConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading phase dependency config %s: %w", configPath, err)
	}

	return ParseConfig(data)
}

// ParseConfig parses YAML bytes into a PhaseDependencyConfig.
func ParseConfig(data []byte) (*PhaseDependencyConfig, error) {
	var cfg PhaseDependencyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing phase dependency config: %w", err)
	}

	if cfg.Dependencies == nil {
		cfg.Dependencies = make(map[string]map[string]LoadStrategy)
	}

	if cfg.V1ToV2 == nil {
		cfg.V1ToV2 = make(map[string]string)
	}

	// Validate strategies
	for phase, deps := range cfg.Dependencies {
		for dep, strategy := range deps {
			if strategy != Full && strategy != Summary {
				return nil, fmt.Errorf(
					"invalid load strategy %q for dependency %s->%s (must be %q or %q)",
					strategy, phase, dep, Full, Summary,
				)
			}
		}
	}

	return &cfg, nil
}

// GetDependencies returns the dependency map for a given phase.
// If the phase is a V1 name, it is resolved to V2 first.
// Returns an empty map if the phase has no dependencies.
func (c *PhaseDependencyConfig) GetDependencies(phase string) map[string]LoadStrategy {
	resolved := c.ResolveV1Name(phase)

	deps, ok := c.Dependencies[resolved]
	if !ok {
		return make(map[string]LoadStrategy)
	}

	// Return a copy to prevent mutation
	result := make(map[string]LoadStrategy, len(deps))
	for k, v := range deps {
		result[k] = v
	}

	return result
}

// ResolveV1Name maps a V1 phase name (e.g. "D1") to its V2 equivalent
// (e.g. "PROBLEM"). If the name is not a V1 name, it is returned unchanged.
func (c *PhaseDependencyConfig) ResolveV1Name(v1Name string) string {
	if v2, ok := c.V1ToV2[v1Name]; ok {
		return v2
	}

	return v1Name
}

// Phases returns all V2 phase names defined in the dependency graph.
func (c *PhaseDependencyConfig) Phases() []string {
	phases := make([]string, 0, len(c.Dependencies))
	for phase := range c.Dependencies {
		phases = append(phases, phase)
	}

	return phases
}
