// Package enforcement provides shared violation detection and enforcement
// utilities used by both the Astrocyte daemon and engram PreToolUse hooks.
//
// It consolidates pattern matching rules so they are defined once and
// consumed by multiple enforcement layers.
package enforcement

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Pattern represents a single anti-pattern rule for violation detection.
type Pattern struct {
	ID              string   `yaml:"id"`
	Order           int      `yaml:"order"`
	RE2Regex        string   `yaml:"re2_regex"`
	Regex           string   `yaml:"regex"`
	PatternName     string   `yaml:"pattern_name"`
	Remediation     string   `yaml:"remediation"`
	Reason          string   `yaml:"reason"`
	Alternative     string   `yaml:"alternative"`
	Examples        []string `yaml:"examples"`
	ShouldNotMatch  []string `yaml:"should_not_match"`
	Severity        string   `yaml:"severity"`
	Tier1Example    string   `yaml:"tier1_example"`
	Tier2Validation bool     `yaml:"tier2_validation"`
	Tier3Rejection  bool     `yaml:"tier3_rejection"`
	ContextCheck    string   `yaml:"context_check"`

	// Consolidation and relaxation metadata.
	ConsolidatedInto string `yaml:"consolidated_into"`
	Relaxed          bool   `yaml:"relaxed"`
	RelaxedDate      string `yaml:"relaxed_date"`
	RelaxedReason    string `yaml:"relaxed_reason"`
}

// PatternDatabase represents a collection of patterns loaded from a YAML file.
type PatternDatabase struct {
	Version  string    `yaml:"version"`
	Updated  string    `yaml:"updated"`
	Purpose  string    `yaml:"purpose"`
	UsedBy   []string  `yaml:"used_by"`
	Patterns []Pattern `yaml:"patterns"`
}

// LoadPatterns loads patterns from a YAML file.
func LoadPatterns(path string) (*PatternDatabase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pattern file %s: %w", path, err)
	}

	var db PatternDatabase
	if err := yaml.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("failed to parse pattern YAML from %s: %w", path, err)
	}

	if len(db.Patterns) == 0 {
		return nil, fmt.Errorf("no patterns found in %s", path)
	}

	return &db, nil
}

// LoadPatternsByType loads patterns for a specific type (bash, beads, git).
// It looks for pattern files in the standard location:
// ~/src/ws/oss/repos/engram/patterns/{type}-anti-patterns.yaml
func LoadPatternsByType(patternType string) (*PatternDatabase, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	path := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns",
		fmt.Sprintf("%s-anti-patterns.yaml", patternType))

	return LoadPatterns(path)
}

// GetPattern returns a pattern by ID, or nil if not found.
func (db *PatternDatabase) GetPattern(id string) *Pattern {
	for i := range db.Patterns {
		if db.Patterns[i].ID == id {
			return &db.Patterns[i]
		}
	}
	return nil
}

// FilterBySeverity returns patterns matching the specified severity level.
func (db *PatternDatabase) FilterBySeverity(severity string) []Pattern {
	var filtered []Pattern
	for _, p := range db.Patterns {
		if p.Severity == severity {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// FilterByTier returns patterns enabled for the specified tier ("tier2" or "tier3").
func (db *PatternDatabase) FilterByTier(tier string) []Pattern {
	var filtered []Pattern
	for _, p := range db.Patterns {
		switch tier {
		case "tier2":
			if p.Tier2Validation {
				filtered = append(filtered, p)
			}
		case "tier3":
			if p.Tier3Rejection {
				filtered = append(filtered, p)
			}
		}
	}
	return filtered
}

// ActivePatterns returns patterns that have a non-empty RE2Regex and are not
// relaxed or consolidated. These are the patterns suitable for PreToolUse
// hook enforcement.
func (db *PatternDatabase) ActivePatterns() []Pattern {
	var active []Pattern
	for _, p := range db.Patterns {
		if p.RE2Regex != "" && !p.Relaxed && p.ConsolidatedInto == "" {
			active = append(active, p)
		}
	}
	return active
}
