// Package benchmark provides benchmark functionality.
package benchmark

import (
	"fmt"

	"github.com/vbonnet/dear-agent/internal/common"
)

// calculateStats computes statistical metrics from benchmark timings
func calculateStats(result *BenchmarkResult) error {
	if len(result.Timings) == 0 {
		return fmt.Errorf("no timings available")
	}

	// Convert time.Duration to float64 milliseconds
	values := make([]float64, len(result.Timings))
	for i, d := range result.Timings {
		values[i] = float64(d.Nanoseconds()) / 1e6
	}

	// Calculate statistics using common utilities
	result.MedianMS = common.Median(values)
	result.MeanMS = common.Mean(values)
	result.StddevMS = common.Stddev(values, result.MeanMS)
	result.P95MS = common.Percentile(values, 0.95)
	result.P99MS = common.Percentile(values, 0.99)

	// Min and max
	result.MinMS = values[0]
	result.MaxMS = values[0]
	for _, v := range values {
		if v < result.MinMS {
			result.MinMS = v
		}
		if v > result.MaxMS {
			result.MaxMS = v
		}
	}

	// Coefficient of variation (CV%) = (stddev / mean) * 100
	if result.MeanMS > 0 {
		result.CVPercent = (result.StddevMS / result.MeanMS) * 100
	}

	return nil
}
