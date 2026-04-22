// Package comparison provides comparison functionality.
package comparison

import (
	"fmt"

	"github.com/vbonnet/dear-agent/internal/baseline"
	"github.com/vbonnet/dear-agent/internal/benchmark"
)

// Detector performs regression detection by comparing benchmarks against baselines
type Detector struct {
	Mode DetectionMode
}

// NewDetector creates a new regression detector with the specified mode
func NewDetector(mode DetectionMode) *Detector {
	return &Detector{
		Mode: mode,
	}
}

// Compare compares a benchmark result against a baseline scenario
func (d *Detector) Compare(current *benchmark.BenchmarkResult, baselineScenario *baseline.ScenarioBaseline) (*ComparisonResult, error) {
	if current == nil {
		return nil, fmt.Errorf("current benchmark result is nil")
	}

	if baselineScenario == nil {
		return nil, fmt.Errorf("baseline scenario is nil")
	}

	if baselineScenario.MedianMS == 0 {
		return nil, fmt.Errorf("baseline median is zero (invalid baseline)")
	}

	// Calculate change metrics
	changePercent := ((current.MedianMS - baselineScenario.MedianMS) / baselineScenario.MedianMS) * 100
	changeMultiplier := current.MedianMS / baselineScenario.MedianMS

	// Check thresholds
	localViolation := changeMultiplier > baselineScenario.Thresholds.LocalMultiplier
	ciViolation := changePercent > baselineScenario.Thresholds.CIPercentage
	highVariance := current.CVPercent > baselineScenario.Thresholds.WarningCV

	// Determine if this is a regression based on detection mode
	var isRegression bool
	switch d.Mode {
	case ModeLocal:
		isRegression = localViolation
	case ModeCI:
		isRegression = ciViolation
	case ModeBoth:
		isRegression = localViolation || ciViolation
	default:
		return nil, fmt.Errorf("unknown detection mode: %s", d.Mode)
	}

	return &ComparisonResult{
		Scenario:           current.Scenario,
		BaselineMedianMS:   baselineScenario.MedianMS,
		CurrentMedianMS:    current.MedianMS,
		ChangePercent:      changePercent,
		ChangeMultiplier:   changeMultiplier,
		LocalThreshold:     baselineScenario.Thresholds.LocalMultiplier,
		CIThreshold:        baselineScenario.Thresholds.CIPercentage,
		IsRegression:       isRegression,
		LocalViolation:     localViolation,
		CIViolation:        ciViolation,
		HighVariance:       highVariance,
		CurrentCVPercent:   current.CVPercent,
		WarningCVThreshold: baselineScenario.Thresholds.WarningCV,
	}, nil
}

// CompareMultiple compares multiple benchmark results against their baselines
func (d *Detector) CompareMultiple(results []*benchmark.BenchmarkResult, mgr *baseline.Manager) ([]*ComparisonResult, error) {
	comparisons := make([]*ComparisonResult, 0, len(results))

	for _, result := range results {
		baselineScenario, err := mgr.GetScenario(result.Scenario)
		if err != nil {
			return nil, fmt.Errorf("failed to get baseline for scenario %q: %w", result.Scenario, err)
		}

		comparison, err := d.Compare(result, baselineScenario)
		if err != nil {
			return nil, fmt.Errorf("failed to compare scenario %q: %w", result.Scenario, err)
		}

		comparisons = append(comparisons, comparison)
	}

	return comparisons, nil
}

// HasRegressions returns true if any comparison shows a regression
func HasRegressions(comparisons []*ComparisonResult) bool {
	for _, c := range comparisons {
		if c.IsRegression {
			return true
		}
	}
	return false
}

// HasHighVariance returns true if any comparison shows high variance
func HasHighVariance(comparisons []*ComparisonResult) bool {
	for _, c := range comparisons {
		if c.HighVariance {
			return true
		}
	}
	return false
}

// GetRegressions returns only the comparisons that show regressions
func GetRegressions(comparisons []*ComparisonResult) []*ComparisonResult {
	regressions := make([]*ComparisonResult, 0)
	for _, c := range comparisons {
		if c.IsRegression {
			regressions = append(regressions, c)
		}
	}
	return regressions
}
