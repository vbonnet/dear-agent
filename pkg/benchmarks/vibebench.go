package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// VibeBench runs the Replit Vibe Bench suite — end-to-end app generation.
// Tasks present a product brief; the agent must generate a runnable app and
// the evaluator checks both functional completeness and "vibes" (UX polish).
// Failure analysis distinguishes structural failures (does not run) from
// behavioral failures (runs but missing features).
type VibeBench struct {
	Loader   TaskLoader
	Executor TaskExecutor
}

// NewVibeBench constructs a VibeBench. nil values fall back to the built-in
// fixture loader and the StubExecutor.
func NewVibeBench(loader TaskLoader, executor TaskExecutor) *VibeBench {
	if loader == nil {
		loader = builtinLoader{suite: SuiteVibeBench, tasks: vibeBenchFixture()}
	}
	if executor == nil {
		executor = StubExecutor{}
	}
	return &VibeBench{Loader: loader, Executor: executor}
}

// Suite reports the benchmark suite identifier.
func (s *VibeBench) Suite() Suite { return SuiteVibeBench }

// Run executes the suite end-to-end.
func (s *VibeBench) Run(ctx context.Context, cfg RunConfig) (*Results, error) {
	return runTasks(ctx, runOptions{
		suite:    SuiteVibeBench,
		model:    cfg.Model,
		mode:     cfg.Mode,
		loader:   s.Loader,
		executor: s.Executor,
		limit:    cfg.Limit,
		budget:   cfg.BudgetUSD,
		tags:     cfg.Tags,
	})
}

// Analyze surfaces failure patterns specific to end-to-end app generation.
func (s *VibeBench) Analyze(results *Results) (*Insights, error) {
	if results == nil {
		return nil, fmt.Errorf("benchmarks: Analyze requires non-nil results")
	}

	patterns := map[string]*FailurePattern{
		"build-failure": {
			Name:       "build-failure",
			Hypothesis: "Generated app does not build — strengthen DEAR Audit phase to run the build before reporting Done",
		},
		"runtime-error": {
			Name:       "runtime-error",
			Hypothesis: "App builds but crashes at runtime — add a smoke-test acceptance criterion",
		},
		"missing-feature": {
			Name:       "missing-feature",
			Hypothesis: "App runs but misses brief features — Define phase under-decomposes the brief; add a feature checklist",
		},
		"vibes-only": {
			Name:       "vibes-only",
			Hypothesis: "Functionally complete but UX is rough — Reflect phase should drive a styling pass for vibe-graded tasks",
		},
	}

	for _, t := range results.Tasks {
		if t.Solved {
			continue
		}
		key := vibeFailureKey(t)
		classify(patterns, key, t.TaskID)
	}

	out := make([]FailurePattern, 0, len(patterns))
	for _, p := range patterns {
		if p.Count > 0 {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })

	return &Insights{
		Suite:    SuiteVibeBench,
		RunID:    results.RunID,
		Patterns: out,
	}, nil
}

// Compare diffs two Vibe Bench runs and returns the Delta.
func (s *VibeBench) Compare(baseline, test *Results) (*Delta, error) {
	return ComputeDelta(baseline, test)
}

func vibeFailureKey(t TaskResult) string {
	if t.Error != "" {
		return "build-failure"
	}
	note, _ := t.Metadata["evaluator_note"].(string)
	switch lower := strings.ToLower(note); {
	case strings.Contains(lower, "runtime"), strings.Contains(lower, "crash"):
		return "runtime-error"
	case strings.Contains(lower, "missing"):
		return "missing-feature"
	case strings.Contains(lower, "vibe"), strings.Contains(lower, "style"):
		return "vibes-only"
	default:
		return "missing-feature"
	}
}
