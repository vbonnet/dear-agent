package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader handles preset loading with inheritance resolution
type Loader struct {
	presetDir string
	cache     map[string]*Preset
	loadChain []string // Track inheritance chain for cycle detection
}

// NewLoader creates a preset loader with default directory
func NewLoader() *Loader {
	homeDir, _ := os.UserHomeDir()
	return &Loader{
		presetDir: filepath.Join(homeDir, ".wayfinder", "presets"),
		cache:     make(map[string]*Preset),
	}
}

// NewLoaderWithDir creates a preset loader with custom directory
func NewLoaderWithDir(dir string) *Loader {
	return &Loader{
		presetDir: dir,
		cache:     make(map[string]*Preset),
	}
}

// Load preset with inheritance resolution
func (l *Loader) Load(name string) (*Preset, error) {
	// Check cache
	if cached, ok := l.cache[name]; ok {
		return cached, nil
	}

	// Reset load chain for new load
	l.loadChain = []string{}

	// Load with inheritance
	preset, err := l.loadWithInheritance(name)
	if err != nil {
		return nil, err
	}

	return preset, nil
}

func (l *Loader) loadWithInheritance(name string) (*Preset, error) {
	// Detect circular inheritance (DEC-007)
	if slices.Contains(l.loadChain, name) {
		chain := strings.Join(append(l.loadChain, name), " → ")
		return nil, fmt.Errorf("circular inheritance detected: %s", chain)
	}

	// Check max depth (DEC-007: ≤5 levels)
	if len(l.loadChain) >= 5 {
		return nil, fmt.Errorf("inheritance depth exceeds maximum (5 levels)")
	}

	// Load preset file
	path := filepath.Join(l.presetDir, name+".yaml")
	preset, err := l.loadFile(path)
	if err != nil {
		return nil, err
	}

	// Validate preset
	if err := l.validatePreset(preset); err != nil {
		return nil, fmt.Errorf("validation failed for %s: %w", name, err)
	}

	// If no inheritance, return as-is
	if preset.Extends == "" {
		l.cache[name] = preset
		return preset, nil
	}

	// Recursive: Load base preset
	l.loadChain = append(l.loadChain, name)
	basePreset, err := l.loadWithInheritance(preset.Extends)
	l.loadChain = l.loadChain[:len(l.loadChain)-1]

	if err != nil {
		return nil, fmt.Errorf("failed to load base preset '%s': %w", preset.Extends, err)
	}

	// Merge: Apply overrides onto base
	merged := l.mergePresets(basePreset, preset)
	l.cache[name] = merged
	return merged, nil
}

// Load preset file (with size check)
func (l *Loader) loadFile(path string) (*Preset, error) {
	// File size check (DEC-012: ≤100KB)
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("preset file not found: %s", path)
		}
		return nil, err
	}

	if fileInfo.Size() > 100*1024 {
		return nil, fmt.Errorf("preset file exceeds maximum size (100KB): %d bytes", fileInfo.Size())
	}

	// Read YAML
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse YAML
	var preset Preset
	if err := yaml.Unmarshal(data, &preset); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	return &preset, nil
}

// validatePreset performs basic validation (DEC-009 Layer 1+2)
func (l *Loader) validatePreset(preset *Preset) error {
	// Validate preset name (DEC-011)
	if !isValidPresetName(preset.Name) {
		return fmt.Errorf("invalid preset name '%s': use lowercase letters, digits, and hyphens only", preset.Name)
	}

	// Reserved names
	reserved := []string{"none", "default", "auto"}
	if slices.Contains(reserved, preset.Name) {
		return fmt.Errorf("preset name '%s' is reserved", preset.Name)
	}

	// Range validation
	if preset.TestCoverage.MinimumPercentage < 0 || preset.TestCoverage.MinimumPercentage > 100 {
		return fmt.Errorf("test_coverage.minimum_percentage must be 0-100, got %d",
			preset.TestCoverage.MinimumPercentage)
	}

	if preset.TestCoverage.MinimumTestCount < 0 {
		return fmt.Errorf("test_coverage.minimum_test_count must be ≥0, got %d",
			preset.TestCoverage.MinimumTestCount)
	}

	if preset.EconomicTuning.ReputationMultiplier <= 0 {
		return fmt.Errorf("economic_tuning.reputation_multiplier must be >0, got %f",
			preset.EconomicTuning.ReputationMultiplier)
	}

	if preset.EconomicTuning.TokenCostMultiplier <= 0 {
		return fmt.Errorf("economic_tuning.token_cost_multiplier must be >0, got %f",
			preset.EconomicTuning.TokenCostMultiplier)
	}

	return nil
}

// isValidPresetName validates preset name (DEC-011)
func isValidPresetName(name string) bool {
	// Pattern: ^[a-z0-9-]+$
	matched, _ := regexp.MatchString(`^[a-z0-9-]+$`, name)
	if !matched {
		return false
	}

	// Max length 64
	return len(name) <= 64
}

// Merge base preset with overrides
func (l *Loader) mergePresets(base *Preset, override *Preset) *Preset {
	// Deep copy base
	result := *base

	// Update metadata from override
	result.Name = override.Name
	result.Description = override.Description

	// Apply overrides if present
	if override.Overrides == nil {
		return &result
	}

	mergeTestCoverageOverrides(&result, override)
	mergeSpecAlignmentOverrides(&result, override)
	mergePhaseGatesOverrides(&result, override)
	mergeRetrospectiveOverrides(&result, override)
	mergeEconomicTuningOverrides(&result, override)

	return &result
}

// mergeTestCoverageOverrides applies test coverage overrides to result
func mergeTestCoverageOverrides(result *Preset, override *Preset) {
	if o := override.Overrides.TestCoverage; o != nil {
		if o.MinimumPercentage != nil {
			result.TestCoverage.MinimumPercentage = *o.MinimumPercentage
		}
		if o.MinimumTestCount != nil {
			result.TestCoverage.MinimumTestCount = *o.MinimumTestCount
		}
		if o.EnforceCreativeTests != nil {
			result.TestCoverage.EnforceCreativeTests = *o.EnforceCreativeTests
		}
	}
}

// mergeSpecAlignmentOverrides applies spec alignment overrides to result
func mergeSpecAlignmentOverrides(result *Preset, override *Preset) {
	if o := override.Overrides.SpecAlignment; o != nil {
		if o.AllowDrift != nil {
			result.SpecAlignment.AllowDrift = *o.AllowDrift
		}
		if o.CheckpointAuditorStrictness != nil {
			result.SpecAlignment.CheckpointAuditorStrictness = *o.CheckpointAuditorStrictness
		}
		if o.FreezeDuringBuild != nil {
			result.SpecAlignment.FreezeDuringBuild = *o.FreezeDuringBuild
		}
	}
}

// mergePhaseGatesOverrides applies phase gates overrides to result
func mergePhaseGatesOverrides(result *Preset, override *Preset) {
	if o := override.Overrides.PhaseGates; o != nil {
		if o.S8BuildVerification != nil {
			result.PhaseGates.S8BuildVerification = *o.S8BuildVerification
		}
		if o.S9ValidationDepth != nil {
			result.PhaseGates.S9ValidationDepth = *o.S9ValidationDepth
		}
		if o.S9HaltOnMinorIssues != nil {
			result.PhaseGates.S9HaltOnMinorIssues = *o.S9HaltOnMinorIssues
		}
		if o.DeployGate != nil {
			result.PhaseGates.DeployGate = *o.DeployGate
		}
	}
}

// mergeRetrospectiveOverrides applies retrospective overrides to result
func mergeRetrospectiveOverrides(result *Preset, override *Preset) {
	if o := override.Overrides.Retrospective; o != nil {
		if o.Mandatory != nil {
			result.Retrospective.Mandatory = *o.Mandatory
		}
		if o.StructuredLearnings != nil {
			result.Retrospective.StructuredLearnings = *o.StructuredLearnings
		}
	}
}

// mergeEconomicTuningOverrides applies economic tuning overrides to result
func mergeEconomicTuningOverrides(result *Preset, override *Preset) {
	if o := override.Overrides.EconomicTuning; o != nil {
		if o.ReputationMultiplier != nil {
			result.EconomicTuning.ReputationMultiplier = *o.ReputationMultiplier
		}
		if o.TokenCostMultiplier != nil {
			result.EconomicTuning.TokenCostMultiplier = *o.TokenCostMultiplier
		}
		if o.PenaltySeverity != nil {
			result.EconomicTuning.PenaltySeverity = *o.PenaltySeverity
		}
	}
}

// ListAvailable returns list of available presets
func (l *Loader) ListAvailable() []string {
	files, err := os.ReadDir(l.presetDir)
	if err != nil {
		return []string{"high-quality", "fast-iteration", "research-heavy"} // Fallback
	}

	var presets []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
			name := strings.TrimSuffix(file.Name(), ".yaml")
			presets = append(presets, name)
		}
	}

	return presets
}
