// Package phasegraph provides phase dependency graph configuration
// with full and summary loading strategies for canonical V2 phases.
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

// PhaseDependencyConfig holds named-phase dependency loading strategies.
type PhaseDependencyConfig struct {
	Dependencies map[string]map[string]LoadStrategy `yaml:"dependencies"`
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
// Returns an empty map if the phase has no dependencies.
func (c *PhaseDependencyConfig) GetDependencies(phase string) map[string]LoadStrategy {
	deps, ok := c.Dependencies[phase]
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

// Phases returns all named phases defined in the dependency graph.
func (c *PhaseDependencyConfig) Phases() []string {
	phases := make([]string, 0, len(c.Dependencies))
	for phase := range c.Dependencies {
		phases = append(phases, phase)
	}

	return phases
}
