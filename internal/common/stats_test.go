package common

import (
	"math"
	"testing"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"empty slice", []float64{}, 0},
		{"single value", []float64{5.0}, 5.0},
		{"odd count", []float64{1, 3, 5}, 3.0},
		{"even count", []float64{1, 2, 3, 4}, 2.5},
		{"unsorted", []float64{5, 1, 3}, 3.0},
		{"duplicates", []float64{2, 2, 2}, 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Median(tt.values)
			if got != tt.want {
				t.Errorf("Median() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"empty slice", []float64{}, 0},
		{"single value", []float64{5.0}, 5.0},
		{"multiple values", []float64{1, 2, 3, 4, 5}, 3.0},
		{"decimals", []float64{1.5, 2.5, 3.5}, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Mean(tt.values)
			if got != tt.want {
				t.Errorf("Mean() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStddev(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		mean   float64
		want   float64
	}{
		{"empty slice", []float64{}, 0, 0},
		{"single value", []float64{5.0}, 5.0, 0},
		{"no variance", []float64{3, 3, 3}, 3.0, 0},
		{"simple case", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 5.0, 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Stddev(tt.values, tt.mean)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("Stddev() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		name string
		p    float64
		want float64
	}{
		{"p0 (min)", 0.0, 1.0},
		{"p50 (median)", 0.50, 5.5},
		{"p95", 0.95, 9.55},
		{"p99", 0.99, 9.91},
		{"p100 (max)", 1.0, 10.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Percentile(values, tt.p)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("Percentile(%v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}

	// Test edge cases
	t.Run("empty slice", func(t *testing.T) {
		got := Percentile([]float64{}, 0.5)
		if got != 0 {
			t.Errorf("Percentile(empty, 0.5) = %v, want 0", got)
		}
	})
}
