package telemetry

import (
	"testing"
	"time"
)

func TestRunControlVariantBenchmark(t *testing.T) {
	// Use 50ms base with 20% overhead (10ms difference) to be well above
	// system scheduling noise and integer millisecond rounding.
	controlFn := func() error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	experimentFn := func() error {
		time.Sleep(60 * time.Millisecond) // 60ms = 20% overhead (well above noise)
		return nil
	}

	result, err := RunControlVariantBenchmark(controlFn, experimentFn, 30)
	if err != nil {
		t.Fatalf("RunControlVariantBenchmark() error = %v", err)
	}

	if result.SampleSize != 30 {
		t.Errorf("SampleSize = %d, want 30", result.SampleSize)
	}

	if result.ControlMean <= 0 {
		t.Errorf("ControlMean = %.2f, want > 0", result.ControlMean)
	}

	// With 20% overhead and 50ms base, experiment should always be measurably larger
	if result.ExperimentMean <= result.ControlMean {
		t.Errorf("ExperimentMean = %.2f should be > ControlMean %.2f", result.ExperimentMean, result.ControlMean)
	}

	// Overhead should be around 20% (allowing wide variance for system load)
	if result.OverheadPercent < 10.0 || result.OverheadPercent > 30.0 {
		t.Errorf("OverheadPercent = %.2f%%, expected 10-30%% (20%% nominal)", result.OverheadPercent)
	}

	if result.StatisticalPower < 0.75 {
		t.Errorf("StatisticalPower = %.2f, want >= 0.75 for n=30", result.StatisticalPower)
	}

	// With 20% overhead, regression should always be detected (threshold is 5%)
	if !result.RegressionDetected {
		t.Errorf("RegressionDetected = false, want true (overhead = %.2f%%)", result.OverheadPercent)
	}
}

func TestRegressionDetection(t *testing.T) {
	// Use 50ms base delays. Differences must be large enough to survive
	// system scheduling jitter (~1-2ms) and integer millisecond rounding.
	// Avoid testing near the 5% threshold — that's inherently flaky.
	tests := []struct {
		name            string
		controlDelay    time.Duration
		experimentDelay time.Duration
		wantRegression  bool
	}{
		{
			name:            "No regression (2% overhead)",
			controlDelay:    50 * time.Millisecond,
			experimentDelay: 51 * time.Millisecond, // 51ms = 2% — well below 5% threshold
			wantRegression:  false,
		},
		{
			name:            "Regression detected (20% overhead)",
			controlDelay:    50 * time.Millisecond,
			experimentDelay: 60 * time.Millisecond, // 60ms = 20% — well above 5% threshold
			wantRegression:  true,
		},
		{
			name:            "Clear regression (10% overhead)",
			controlDelay:    50 * time.Millisecond,
			experimentDelay: 55 * time.Millisecond, // 55ms = 10% — safely above 5% threshold
			wantRegression:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlFn := func() error {
				time.Sleep(tt.controlDelay)
				return nil
			}
			experimentFn := func() error {
				time.Sleep(tt.experimentDelay)
				return nil
			}

			result, err := RunControlVariantBenchmark(controlFn, experimentFn, 30)
			if err != nil {
				t.Fatalf("RunControlVariantBenchmark() error = %v", err)
			}

			if result.RegressionDetected != tt.wantRegression {
				t.Errorf("RegressionDetected = %v, want %v (overhead = %.2f%%)",
					result.RegressionDetected, tt.wantRegression, result.OverheadPercent)
			}
		})
	}
}

func TestEstimatePower(t *testing.T) {
	tests := []struct {
		sampleSize int
		minPower   float64
		maxPower   float64
	}{
		{sampleSize: 10, minPower: 0.35, maxPower: 0.45},
		{sampleSize: 30, minPower: 0.75, maxPower: 0.85},
		{sampleSize: 50, minPower: 0.85, maxPower: 0.95},
		{sampleSize: 100, minPower: 0.90, maxPower: 1.00},
	}

	for _, tt := range tests {
		power := estimatePower(tt.sampleSize)
		if power < tt.minPower || power > tt.maxPower {
			t.Errorf("estimatePower(%d) = %.2f, want [%.2f, %.2f]",
				tt.sampleSize, power, tt.minPower, tt.maxPower)
		}
	}
}

func TestBenchmarkSampleSizeValidation(t *testing.T) {
	controlFn := func() error { return nil }
	experimentFn := func() error { return nil }

	_, err := RunControlVariantBenchmark(controlFn, experimentFn, 5)
	if err == nil {
		t.Error("Expected error for sample size < 10, got nil")
	}
}
