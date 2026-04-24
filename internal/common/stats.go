package common

import (
	"math"
	"sort"
)

// Median calculates the median value from a slice of float64 values.
// Returns 0 for empty slices.
func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Sort a copy to avoid mutating original
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		// Even number: average of two middle values
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	// Odd number: middle value
	return sorted[n/2]
}

// Mean calculates the arithmetic mean (average) of values.
// Returns 0 for empty slices.
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// Stddev calculates the standard deviation given values and their mean.
// Returns 0 for empty slices or single value.
func Stddev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}

	var sumSquaredDiff float64
	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}

	variance := sumSquaredDiff / float64(len(values))
	return math.Sqrt(variance)
}

// Percentile calculates the value at the given percentile (0.0 to 1.0).
// Uses linear interpolation between closest ranks.
// Returns 0 for empty slices.
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Sort a copy
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := float64(len(sorted))
	rank := p * (n - 1)
	lowerIndex := int(math.Floor(rank))
	upperIndex := int(math.Ceil(rank))

	if lowerIndex == upperIndex {
		return sorted[lowerIndex]
	}

	// Linear interpolation
	fraction := rank - float64(lowerIndex)
	return sorted[lowerIndex]*(1-fraction) + sorted[upperIndex]*fraction
}
