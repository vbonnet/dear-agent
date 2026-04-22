// Package baseline provides baseline functionality.
package baseline

import (
	"fmt"
	"time"
)

const (
	// CurrentSchemaVersion is the version of the baseline schema
	CurrentSchemaVersion = "1.0"
)

// Baseline represents the complete baseline file structure
type Baseline struct {
	SchemaVersion string                       `json:"schema_version"`
	CreatedAt     time.Time                    `json:"created_at"`
	UpdatedAt     time.Time                    `json:"updated_at"`
	GitCommit     string                       `json:"git_commit"`
	GitBranch     string                       `json:"git_branch"`
	Scenarios     map[string]*ScenarioBaseline `json:"scenarios"`
}

// ScenarioBaseline contains baseline data for a specific test scenario
type ScenarioBaseline struct {
	Scenario   string         `json:"scenario"`
	MedianMS   float64        `json:"median_ms"`
	MeanMS     float64        `json:"mean_ms"`
	StddevMS   float64        `json:"stddev_ms"`
	P95MS      float64        `json:"p95_ms"`
	P99MS      float64        `json:"p99_ms"`
	CVPercent  float64        `json:"cv_percent"`
	Runs       int            `json:"runs"`
	Thresholds *Thresholds    `json:"thresholds"`
	History    []HistoryEntry `json:"history,omitempty"`
}

// Thresholds defines regression detection thresholds
type Thresholds struct {
	LocalMultiplier float64 `json:"local_multiplier"` // e.g., 2.0 for "2x slower blocks commit"
	CIPercentage    float64 `json:"ci_percentage"`    // e.g., 15.0 for "15% slower fails CI"
	WarningCV       float64 `json:"warning_cv"`       // e.g., 20.0 for "CV% > 20% warns"
}

// HistoryEntry records a baseline update event
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	GitCommit string    `json:"git_commit"`
	GitBranch string    `json:"git_branch"`
	MedianMS  float64   `json:"median_ms"`
	Reason    string    `json:"reason"`
}

// NewBaseline creates a new baseline with default values
func NewBaseline() *Baseline {
	now := time.Now()
	return &Baseline{
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
		Scenarios:     make(map[string]*ScenarioBaseline),
	}
}

// NewScenarioBaseline creates a new scenario baseline with default thresholds
func NewScenarioBaseline(scenario string) *ScenarioBaseline {
	return &ScenarioBaseline{
		Scenario: scenario,
		Thresholds: &Thresholds{
			LocalMultiplier: 2.0,  // Default: block at >2x
			CIPercentage:    15.0, // Default: fail CI at >15%
			WarningCV:       20.0, // Default: warn at >20% CV%
		},
		History: []HistoryEntry{},
	}
}

// ValidateSchema checks if the baseline has a valid schema and required fields
func ValidateSchema(b *Baseline) error {
	if b == nil {
		return fmt.Errorf("baseline is nil")
	}

	if b.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}

	if b.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schema version %q (expected %q)",
			b.SchemaVersion, CurrentSchemaVersion)
	}

	if b.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}

	if b.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required")
	}

	if b.Scenarios == nil {
		return fmt.Errorf("scenarios map is required")
	}

	// Validate each scenario
	for name, scenario := range b.Scenarios {
		if scenario == nil {
			return fmt.Errorf("scenario %q is nil", name)
		}

		if scenario.Scenario == "" {
			return fmt.Errorf("scenario %q: scenario name is required", name)
		}

		if scenario.MedianMS < 0 {
			return fmt.Errorf("scenario %q: median_ms cannot be negative", name)
		}

		if scenario.Runs < 1 {
			return fmt.Errorf("scenario %q: runs must be >= 1", name)
		}

		if scenario.Thresholds == nil {
			return fmt.Errorf("scenario %q: thresholds are required", name)
		}

		if err := validateThresholds(name, scenario.Thresholds); err != nil {
			return err
		}
	}

	return nil
}

// validateThresholds checks if threshold values are valid
func validateThresholds(scenarioName string, t *Thresholds) error {
	if t.LocalMultiplier <= 1.0 {
		return fmt.Errorf("scenario %q: local_multiplier must be > 1.0 (got %.2f)",
			scenarioName, t.LocalMultiplier)
	}

	if t.CIPercentage <= 0 || t.CIPercentage > 100 {
		return fmt.Errorf("scenario %q: ci_percentage must be between 0 and 100 (got %.2f)",
			scenarioName, t.CIPercentage)
	}

	if t.WarningCV <= 0 || t.WarningCV > 100 {
		return fmt.Errorf("scenario %q: warning_cv must be between 0 and 100 (got %.2f)",
			scenarioName, t.WarningCV)
	}

	return nil
}
