package benchmarks

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ComputeSummary aggregates a slice of TaskResult into a Summary. It is
// deterministic: callers can call it multiple times on the same input and
// get the same Summary.
func ComputeSummary(tasks []TaskResult) Summary {
	s := Summary{Total: len(tasks)}
	var totalDur time.Duration
	for _, t := range tasks {
		switch {
		case t.Error != "":
			s.Errored++
		case t.Solved:
			s.Solved++
		default:
			s.Failed++
		}
		s.TotalCostUSD += t.CostUSD
		s.TotalTokensIn += t.TokensIn
		s.TotalTokensOut += t.TokensOut
		totalDur += t.Duration
	}
	if s.Total > 0 {
		s.SolveRate = float64(s.Solved) / float64(s.Total)
		s.AvgDuration = totalDur / time.Duration(s.Total)
	}
	if s.Solved > 0 {
		s.CostPerSolved = s.TotalCostUSD / float64(s.Solved)
	} else {
		s.CostPerSolved = math.Inf(1)
	}
	return s
}

// ComputeDelta diffs two Results and returns the per-task and aggregate
// changes. baseline and test must be runs of the same Suite.
func ComputeDelta(baseline, test *Results) (*Delta, error) {
	if baseline == nil || test == nil {
		return nil, fmt.Errorf("benchmarks: ComputeDelta requires non-nil baseline and test")
	}
	if baseline.Suite != test.Suite {
		return nil, fmt.Errorf("benchmarks: cannot compare runs of different suites: %q vs %q",
			baseline.Suite, test.Suite)
	}

	baselineSolved := indexSolved(baseline.Tasks)
	testSolved := indexSolved(test.Tasks)

	var regressions, improvements []string
	for id := range baselineSolved {
		if !testSolved[id] {
			regressions = append(regressions, id)
		}
	}
	for id := range testSolved {
		if !baselineSolved[id] {
			improvements = append(improvements, id)
		}
	}
	sort.Strings(regressions)
	sort.Strings(improvements)

	d := &Delta{
		Suite:              baseline.Suite,
		BaselineRunID:      baseline.RunID,
		TestRunID:          test.RunID,
		SolveRateDelta:     test.Summary.SolveRate - baseline.Summary.SolveRate,
		CostUSDDelta:       test.Summary.TotalCostUSD - baseline.Summary.TotalCostUSD,
		CostPerSolvedDelta: subFinite(test.Summary.CostPerSolved, baseline.Summary.CostPerSolved),
		Regressions:        regressions,
		Improvements:       improvements,
		IsRegression:       test.Summary.SolveRate < baseline.Summary.SolveRate,
	}
	return d, nil
}

func indexSolved(tasks []TaskResult) map[string]bool {
	out := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.Solved {
			out[t.TaskID] = true
		}
	}
	return out
}

// subFinite returns a-b, treating non-finite operands as 0 so that a delta of
// "neither run solved anything" reads as zero rather than NaN.
func subFinite(a, b float64) float64 {
	if math.IsInf(a, 0) || math.IsNaN(a) {
		a = 0
	}
	if math.IsInf(b, 0) || math.IsNaN(b) {
		b = 0
	}
	return a - b
}
