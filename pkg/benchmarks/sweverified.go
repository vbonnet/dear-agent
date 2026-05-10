package benchmarks

import (
	"context"
	"fmt"
)

// SWEBenchVerified runs the SWE-Bench Verified suite (~500 human-validated
// GitHub-issue → patch tasks). Solve rates here are the headline number for
// "is dear-agent state-of-the-art" claims; runs are more expensive than Lite,
// so this tier is gated as the validation step in the self-improvement loop.
type SWEBenchVerified struct {
	Loader   TaskLoader
	Executor TaskExecutor
}

// NewSWEBenchVerified constructs a SWEBenchVerified. nil values fall back to
// the built-in fixture loader and the StubExecutor.
func NewSWEBenchVerified(loader TaskLoader, executor TaskExecutor) *SWEBenchVerified {
	if loader == nil {
		loader = builtinLoader{suite: SuiteSWEBenchVerified, tasks: sweVerifiedFixture()}
	}
	if executor == nil {
		executor = StubExecutor{}
	}
	return &SWEBenchVerified{Loader: loader, Executor: executor}
}

// Suite reports the benchmark suite identifier.
func (s *SWEBenchVerified) Suite() Suite { return SuiteSWEBenchVerified }

// Run executes the suite end-to-end.
func (s *SWEBenchVerified) Run(ctx context.Context, cfg RunConfig) (*Results, error) {
	return runTasks(ctx, runOptions{
		suite:    SuiteSWEBenchVerified,
		model:    cfg.Model,
		mode:     cfg.Mode,
		loader:   s.Loader,
		executor: s.Executor,
		limit:    cfg.Limit,
		budget:   cfg.BudgetUSD,
		tags:     cfg.Tags,
	})
}

// Analyze surfaces issue → patch failure patterns. SWE-Bench Verified shares
// the Lite taxonomy because the task shape is identical.
func (s *SWEBenchVerified) Analyze(results *Results) (*Insights, error) {
	if results == nil {
		return nil, fmt.Errorf("benchmarks: Analyze requires non-nil results")
	}
	return analyzeIssuePatch(results), nil
}

// Compare diffs two SWE-Bench Verified runs and returns the Delta.
func (s *SWEBenchVerified) Compare(baseline, test *Results) (*Delta, error) {
	return ComputeDelta(baseline, test)
}
