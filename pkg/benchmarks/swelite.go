package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// SWEBenchLite runs the SWE-Bench Lite suite (~300 GitHub-issue → patch tasks).
// It is the cheapest tier and is the recommended starting point for the
// "shift left" optimization loop in pkg/selfimprove: improvements that move
// the SWE-Bench Lite needle are cheap to validate before paying for the
// Verified tier.
type SWEBenchLite struct {
	Loader   TaskLoader
	Executor TaskExecutor
}

// NewSWEBenchLite constructs a SWEBenchLite. nil values fall back to the
// built-in fixture loader and the StubExecutor.
func NewSWEBenchLite(loader TaskLoader, executor TaskExecutor) *SWEBenchLite {
	if loader == nil {
		loader = builtinLoader{suite: SuiteSWEBenchLite, tasks: sweLiteFixture()}
	}
	if executor == nil {
		executor = StubExecutor{}
	}
	return &SWEBenchLite{Loader: loader, Executor: executor}
}

// Suite reports the benchmark suite identifier.
func (s *SWEBenchLite) Suite() Suite { return SuiteSWEBenchLite }

// Run executes the suite end-to-end.
func (s *SWEBenchLite) Run(ctx context.Context, cfg RunConfig) (*Results, error) {
	return runTasks(ctx, runOptions{
		suite:    SuiteSWEBenchLite,
		model:    cfg.Model,
		mode:     cfg.Mode,
		loader:   s.Loader,
		executor: s.Executor,
		limit:    cfg.Limit,
		budget:   cfg.BudgetUSD,
		tags:     cfg.Tags,
	})
}

// Analyze surfaces failure patterns specific to issue → patch tasks. It
// classifies errored vs unsolved tasks by simple keyword heuristics on the
// per-task Error field; richer analysis (e.g. per-repo grouping) can be added
// incrementally.
func (s *SWEBenchLite) Analyze(results *Results) (*Insights, error) {
	if results == nil {
		return nil, fmt.Errorf("benchmarks: Analyze requires non-nil results")
	}
	return analyzeIssuePatch(results), nil
}

// Compare diffs two SWE-Bench Lite runs and returns the Delta.
func (s *SWEBenchLite) Compare(baseline, test *Results) (*Delta, error) {
	return ComputeDelta(baseline, test)
}

// analyzeIssuePatch is shared by SWE-Bench Lite and Verified — both have
// the same task shape (issue + repo + patch) so they share failure
// taxonomies.
func analyzeIssuePatch(results *Results) *Insights {
	patterns := map[string]*FailurePattern{
		"agent-error": {
			Name:       "agent-error",
			Hypothesis: "Agent process failed before producing a patch — investigate prompt scaffolding, timeout, or harness wiring",
		},
		"empty-patch": {
			Name:       "empty-patch",
			Hypothesis: "Agent returned no diff — likely missed the unified-diff instruction or got stuck in exploration; try DEAR Define-phase exit criterion that requires a patch",
		},
		"wrong-file": {
			Name:       "wrong-file",
			Hypothesis: "Patch touched files unrelated to the issue — the agent's repo-search step is under-constrained; consider adding a file-locator audit",
		},
		"test-failure": {
			Name:       "test-failure",
			Hypothesis: "Patch applied but failed the hidden test — the agent did not validate against the issue's test_patch; add a Reflect-phase self-test",
		},
	}

	for _, t := range results.Tasks {
		switch {
		case t.Error != "":
			classify(patterns, "agent-error", t.TaskID)
		case !t.Solved:
			lower := strings.ToLower(fmt.Sprint(t.Metadata["evaluator_note"]))
			switch {
			case strings.Contains(lower, "empty"):
				classify(patterns, "empty-patch", t.TaskID)
			case strings.Contains(lower, "wrong file"):
				classify(patterns, "wrong-file", t.TaskID)
			default:
				classify(patterns, "test-failure", t.TaskID)
			}
		}
	}

	out := make([]FailurePattern, 0, len(patterns))
	for _, p := range patterns {
		if p.Count > 0 {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })

	return &Insights{
		Suite:    results.Suite,
		RunID:    results.RunID,
		Patterns: out,
	}
}

func classify(patterns map[string]*FailurePattern, key, taskID string) {
	p, ok := patterns[key]
	if !ok {
		return
	}
	p.Count++
	if len(p.Examples) < 5 {
		p.Examples = append(p.Examples, taskID)
	}
}
