// Package benchmark provides parsing and evaluation of Go benchmark results.
package benchmark

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BenchmarkResult represents a single parsed Go benchmark result line.
type BenchmarkResult struct {
	Name        string        `json:"name"`
	Iterations  int           `json:"iterations"`
	NsPerOp     float64       `json:"ns_per_op"`
	BytesPerOp  int64         `json:"bytes_per_op,omitempty"`
	AllocsPerOp int64         `json:"allocs_per_op,omitempty"`
	Duration    time.Duration `json:"duration"`
}

// PerformanceTarget defines a threshold for a benchmark.
type PerformanceTarget struct {
	BenchmarkPattern string        `json:"benchmark_pattern"`
	MaxDuration      time.Duration `json:"max_duration"`
	Description      string        `json:"description"`
}

// BenchmarkEvaluation is a result paired with its target comparison.
type BenchmarkEvaluation struct {
	Result BenchmarkResult    `json:"result"`
	Target *PerformanceTarget `json:"target,omitempty"`
	Pass   bool               `json:"pass"`
}

// BenchmarkReport is the top-level output structure.
type BenchmarkReport struct {
	Evaluations []BenchmarkEvaluation `json:"evaluations"`
	Summary     ReportSummary         `json:"summary"`
}

// ReportSummary aggregates pass/fail counts.
type ReportSummary struct {
	Total     int  `json:"total"`
	Passed    int  `json:"passed"`
	Failed    int  `json:"failed"`
	NoTarget  int  `json:"no_target"`
	AllPassed bool `json:"all_passed"`
}

// benchRe matches Go benchmark output lines.
// Format: BenchmarkName-N  iterations  ns/op  [B/op  allocs/op]
var benchRe = regexp.MustCompile(
	`^(Benchmark\S+)\s+(\d+)\s+([\d.]+)\s+ns/op` +
		`(?:\s+(\d+)\s+B/op\s+(\d+)\s+allocs/op)?`,
)

// ParseBenchmarkOutput parses the stdout from `go test -bench=... [-benchmem]`.
func ParseBenchmarkOutput(output string) ([]BenchmarkResult, error) {
	var results []BenchmarkResult

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		m := benchRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		iterations, _ := strconv.Atoi(m[2])
		nsPerOp, _ := strconv.ParseFloat(m[3], 64)

		r := BenchmarkResult{
			Name:       m[1],
			Iterations: iterations,
			NsPerOp:    nsPerOp,
			Duration:   time.Duration(nsPerOp),
		}

		if m[4] != "" {
			r.BytesPerOp, _ = strconv.ParseInt(m[4], 10, 64)
		}
		if m[5] != "" {
			r.AllocsPerOp, _ = strconv.ParseInt(m[5], 10, 64)
		}

		results = append(results, r)
	}

	if len(results) == 0 && strings.Contains(output, "FAIL") {
		return nil, fmt.Errorf("benchmark run failed: no results parsed")
	}

	return results, nil
}

// DefaultTargets returns the performance targets from docs/performance-benchmarks.md.
func DefaultTargets() []PerformanceTarget {
	return []PerformanceTarget{
		{
			BenchmarkPattern: `BenchmarkListSessionsScaled/Sessions_1000`,
			MaxDuration:      100 * time.Millisecond,
			Description:      "List 1000 sessions",
		},
		{
			BenchmarkPattern: `BenchmarkSearchCached`,
			MaxDuration:      1 * time.Millisecond,
			Description:      "Search cache hit",
		},
		{
			BenchmarkPattern: `BenchmarkLockAcquireRelease`,
			MaxDuration:      10 * time.Microsecond,
			Description:      "Lock acquire/release",
		},
	}
}

// Evaluate compares benchmark results against performance targets.
func Evaluate(results []BenchmarkResult, targets []PerformanceTarget) *BenchmarkReport {
	report := &BenchmarkReport{
		Summary: ReportSummary{Total: len(results)},
	}

	for _, r := range results {
		eval := BenchmarkEvaluation{
			Result: r,
			Pass:   true,
		}

		for i := range targets {
			matched, _ := regexp.MatchString(targets[i].BenchmarkPattern, r.Name)
			if matched {
				eval.Target = &targets[i]
				eval.Pass = r.Duration <= targets[i].MaxDuration
				break
			}
		}

		switch {
		case eval.Target == nil:
			report.Summary.NoTarget++
		case eval.Pass:
			report.Summary.Passed++
		default:
			report.Summary.Failed++
		}

		report.Evaluations = append(report.Evaluations, eval)
	}

	report.Summary.AllPassed = report.Summary.Failed == 0
	return report
}
