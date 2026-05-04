package telemetry

import (
	"fmt"
	"math"
	"time"
)

// BenchmarkVariant represents a control or experimental variant in A/B testing.
type BenchmarkVariant string

const (
	// VariantControl is the baseline (telemetry disabled or V0 behavior).
	VariantControl BenchmarkVariant = "control"

	// VariantExperiment is the experimental variant (telemetry enabled or V1 behavior).
	VariantExperiment BenchmarkVariant = "experiment"
)

// BenchmarkRun represents a single benchmark execution.
type BenchmarkRun struct {
	Variant    BenchmarkVariant       `json:"variant"`
	DurationMs int64                  `json:"duration_ms"`
	Successful bool                   `json:"successful"`
	ErrorCount int                    `json:"error_count"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// BenchmarkResult contains statistical comparison between control and experiment.
type BenchmarkResult struct {
	ControlMean        float64 `json:"control_mean_ms"`
	ExperimentMean     float64 `json:"experiment_mean_ms"`
	OverheadPercent    float64 `json:"overhead_percent"`  // (experiment - control) / control * 100
	StatisticalPower   float64 `json:"statistical_power"` // Estimate of test power
	SampleSize         int     `json:"sample_size"`
	RegressionDetected bool    `json:"regression_detected"` // True if overhead ≥ 5%
}

// RunControlVariantBenchmark executes a benchmark comparing control vs experiment.
//
// Executes:
//   - N runs of control variant (telemetry disabled)
//   - N runs of experiment variant (telemetry enabled)
//   - Computes statistical comparison (mean, overhead, power)
//   - Detects regression if overhead ≥ 5% (S9 validation threshold)
//
// Parameters:
//   - controlFn: Function to execute in control mode
//   - experimentFn: Function to execute in experiment mode
//   - sampleSize: Number of runs per variant (minimum 30 for statistical power)
//
// Returns:
//   - BenchmarkResult with statistical analysis
//   - error if benchmark execution fails
func RunControlVariantBenchmark(
	controlFn func() error,
	experimentFn func() error,
	sampleSize int,
) (*BenchmarkResult, error) {
	if sampleSize < 10 {
		return nil, fmt.Errorf("sample size must be ≥10 for meaningful results (got %d)", sampleSize)
	}

	// Run control variant
	controlRuns := executeBenchmarkVariant(controlFn, VariantControl, sampleSize)

	// Run experiment variant
	experimentRuns := executeBenchmarkVariant(experimentFn, VariantExperiment, sampleSize)

	// Compute statistics
	result := computeBenchmarkStatistics(controlRuns, experimentRuns)
	return &result, nil
}

// executeBenchmarkVariant runs a variant N times and collects results.
func executeBenchmarkVariant(fn func() error, variant BenchmarkVariant, n int) []BenchmarkRun {
	runs := make([]BenchmarkRun, n)

	for i := 0; i < n; i++ {
		startTime := time.Now()
		err := fn()
		duration := time.Since(startTime)

		runs[i] = BenchmarkRun{
			Variant:    variant,
			DurationMs: duration.Milliseconds(),
			Successful: err == nil,
			ErrorCount: 0, // Could be enhanced to track error count
		}

		if err != nil {
			runs[i].ErrorCount = 1
		}
	}

	return runs
}

// computeBenchmarkStatistics analyzes control vs experiment results.
func computeBenchmarkStatistics(controlRuns, experimentRuns []BenchmarkRun) BenchmarkResult {
	// Calculate mean durations
	controlMean := calculateMean(controlRuns)
	experimentMean := calculateMean(experimentRuns)

	// Calculate overhead percentage
	overheadPercent := 0.0
	if controlMean > 0 {
		overheadPercent = ((experimentMean - controlMean) / controlMean) * 100
	}

	// Estimate statistical power (simplified)
	// For accurate power analysis, use proper paired t-test with effect size calculation
	sampleSize := len(controlRuns)
	statisticalPower := estimatePower(sampleSize)

	// Detect regression (D4 NFR3: overhead must be <5%)
	regressionDetected := overheadPercent >= 5.0

	return BenchmarkResult{
		ControlMean:        controlMean,
		ExperimentMean:     experimentMean,
		OverheadPercent:    overheadPercent,
		StatisticalPower:   statisticalPower,
		SampleSize:         sampleSize,
		RegressionDetected: regressionDetected,
	}
}

// calculateMean computes average duration from benchmark runs.
func calculateMean(runs []BenchmarkRun) float64 {
	if len(runs) == 0 {
		return 0.0
	}

	sum := int64(0)
	for _, run := range runs {
		sum += run.DurationMs
	}

	return float64(sum) / float64(len(runs))
}

// estimatePower estimates statistical power based on sample size.
//
// Simplified estimation:
//   - n < 10:  power ~0.40 (underpowered)
//   - n = 30:  power ~0.80 (standard threshold)
//   - n = 50:  power ~0.90 (well-powered)
//   - n ≥ 100: power ~0.95 (high power)
//
// For rigorous power analysis, use proper effect size calculation and t-distribution.
func estimatePower(n int) float64 {
	switch {
	case n < 10:
		return 0.40
	case n < 30:
		// Linear interpolation between 0.40 (n=10) and 0.80 (n=30)
		return 0.40 + (float64(n-10) / 20.0 * 0.40)
	case n < 50:
		// Linear interpolation between 0.80 (n=30) and 0.90 (n=50)
		return 0.80 + (float64(n-30) / 20.0 * 0.10)
	case n < 100:
		// Linear interpolation between 0.90 (n=50) and 0.95 (n=100)
		return 0.90 + (float64(n-50) / 50.0 * 0.05)
	default:
		return math.Min(0.99, 0.95+(float64(n-100)/1000.0*0.04))
	}
}
