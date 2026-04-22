package stats

import (
	"math"
	"testing"
)

func TestNewSample(t *testing.T) {
	s := NewSample([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if s.N != 8 {
		t.Errorf("N = %d, want 8", s.N)
	}
	if math.Abs(s.Mean-5.0) > 0.001 {
		t.Errorf("Mean = %f, want 5.0", s.Mean)
	}
	if math.Abs(s.StdDev-2.138) > 0.01 {
		t.Errorf("StdDev = %f, want ~2.138", s.StdDev)
	}
}

func TestNewSampleEmpty(t *testing.T) {
	s := NewSample(nil)
	if s.N != 0 {
		t.Errorf("N = %d, want 0", s.N)
	}
}

func TestWelchTTest_SignificantDifference(t *testing.T) {
	// Two clearly different groups
	a := NewSample([]float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
	b := NewSample([]float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})

	result := WelchTTest(a, b)
	if result.PValue >= 0.05 {
		t.Errorf("PValue = %f, want < 0.05 for clearly different groups", result.PValue)
	}
	if result.T <= 0 {
		t.Errorf("T = %f, want > 0 (a.Mean > b.Mean)", result.T)
	}
}

func TestWelchTTest_NoSignificantDifference(t *testing.T) {
	a := NewSample([]float64{5.0, 5.1, 4.9, 5.0, 5.2})
	b := NewSample([]float64{5.0, 4.9, 5.1, 5.0, 4.8})

	result := WelchTTest(a, b)
	if result.PValue < 0.05 {
		t.Errorf("PValue = %f, want >= 0.05 for similar groups", result.PValue)
	}
}

func TestWelchTTest_InsufficientSamples(t *testing.T) {
	a := NewSample([]float64{5.0})
	b := NewSample([]float64{10.0})

	result := WelchTTest(a, b)
	if result.PValue != 1.0 {
		t.Errorf("PValue = %f, want 1.0 for insufficient samples", result.PValue)
	}
}

func TestEffectSize(t *testing.T) {
	tests := []struct {
		name   string
		a, b   []float64
		wantD  string // "negligible", "small", "medium", "large"
	}{
		{
			name:  "large effect",
			a:     []float64{10, 11, 12, 13, 14},
			b:     []float64{1, 2, 3, 4, 5},
			wantD: "large",
		},
		{
			name:  "negligible effect",
			a:     []float64{5.0, 5.1, 4.9, 5.0, 5.05},
			b:     []float64{5.0, 4.95, 5.1, 5.0, 4.98},
			wantD: "negligible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EffectSize(NewSample(tt.a), NewSample(tt.b))
			if result.Interpretation != tt.wantD {
				t.Errorf("Interpretation = %q, want %q (d=%.3f)", result.Interpretation, tt.wantD, result.D)
			}
		})
	}
}

func TestBootstrapCI(t *testing.T) {
	a := NewSample([]float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
	b := NewSample([]float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})

	ci := BootstrapCI(a, b, 1000, 0.95)
	if ci.Lower <= 0 {
		t.Errorf("CI Lower = %f, want > 0 for clearly different groups", ci.Lower)
	}
	if ci.Upper <= ci.Lower {
		t.Errorf("CI Upper (%f) should be > Lower (%f)", ci.Upper, ci.Lower)
	}
	if ci.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95", ci.Confidence)
	}
}

func TestBootstrapCI_EmptySamples(t *testing.T) {
	a := NewSample(nil)
	b := NewSample([]float64{1, 2, 3})

	ci := BootstrapCI(a, b, 1000, 0.95)
	if ci.Lower != 0 || ci.Upper != 0 {
		t.Errorf("Expected zero CI for empty sample, got [%f, %f]", ci.Lower, ci.Upper)
	}
}

func TestMinSampleSize(t *testing.T) {
	// Medium effect (d=0.5), alpha=0.05, power=0.80
	n := MinSampleSize(0.5, 0.05, 0.80)
	// Expected ~64 per group
	if n < 30 || n > 100 {
		t.Errorf("MinSampleSize(0.5, 0.05, 0.80) = %d, want ~64", n)
	}
}

func TestMinSampleSize_InvalidInputs(t *testing.T) {
	if n := MinSampleSize(0, 0.05, 0.80); n != 0 {
		t.Errorf("Expected 0 for zero effect size, got %d", n)
	}
	if n := MinSampleSize(0.5, 0, 0.80); n != 0 {
		t.Errorf("Expected 0 for zero alpha, got %d", n)
	}
}

func TestIsSignificant(t *testing.T) {
	large := EffectSizeResult{D: 1.0, Interpretation: "large"}
	negligible := EffectSizeResult{D: 0.1, Interpretation: "negligible"}

	if !IsSignificant(0.01, 0.05, large) {
		t.Error("Expected significant for p=0.01, large effect")
	}
	if IsSignificant(0.10, 0.05, large) {
		t.Error("Expected not significant for p=0.10 > alpha")
	}
	if IsSignificant(0.01, 0.05, negligible) {
		t.Error("Expected not significant for negligible effect even with low p")
	}
}
