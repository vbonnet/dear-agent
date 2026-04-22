package benchmark

import "time"

// BenchmarkResult contains timing measurements and statistics from a benchmark run.
type BenchmarkResult struct {
	Command    string          `json:"command"`          // Hook command that was executed
	Scenario   string          `json:"scenario"`         // Test scenario (empty, small, medium)
	Runs       int             `json:"runs"`             // Number of measured runs
	WarmupRuns int             `json:"warmup_runs"`      // Number of warmup runs
	Timings    []time.Duration `json:"-"`                // Raw timings (not serialized)
	MedianMS   float64         `json:"median_ms"`        // Median execution time (ms)
	MeanMS     float64         `json:"mean_ms"`          // Mean execution time (ms)
	StddevMS   float64         `json:"stddev_ms"`        // Standard deviation (ms)
	MinMS      float64         `json:"min_ms"`           // Minimum execution time (ms)
	MaxMS      float64         `json:"max_ms"`           // Maximum execution time (ms)
	P95MS      float64         `json:"p95_ms"`           // 95th percentile (ms)
	P99MS      float64         `json:"p99_ms"`           // 99th percentile (ms)
	CVPercent  float64         `json:"cv_percent"`       // Coefficient of variation (%)
	Errors     []string        `json:"errors,omitempty"` // Errors during runs (if any)
}
