// Package enforcement provides violation detection and enforcement utilities
// for the Astrocyte session monitor and related tools.
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
	Regex           string   `yaml:"regex"`
	Reason          string   `yaml:"reason"`
	Alternative     string   `yaml:"alternative"`
	Examples        []string `yaml:"examples"`
	Severity        string   `yaml:"severity"`
	Tier1Example    string   `yaml:"tier1_example"`
	Tier2Validation bool     `yaml:"tier2_validation"`
	Tier3Rejection  bool     `yaml:"tier3_rejection"`
	ContextCheck    string   `yaml:"context_check"`
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
// Returns an error if the file cannot be read or parsed.
func LoadPatterns(path string) (*PatternDatabase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pattern file %s: %w", path, err)
	}

	var db PatternDatabase
	if err := yaml.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("failed to parse pattern YAML from %s: %w", path, err)
	}

	// Validate that we have at least one pattern
	if len(db.Patterns) == 0 {
		return nil, fmt.Errorf("no patterns found in %s", path)
	}

	return &db, nil
}

// LoadPatternsByType loads patterns for a specific type (bash, beads, git).
// It looks for pattern files in the standard location:
// ~/src/ws/oss/repos/engram/patterns/{type}-anti-patterns.yaml
func LoadPatternsByType(patternType string) (*PatternDatabase, error) {
	// Expand home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Construct path to pattern file
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

// FilterByTier returns patterns enabled for the specified tier.
// tier should be one of: "tier2", "tier3".
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
