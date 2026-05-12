package benchmarks

import (
	"context"
	"fmt"
	"sort"
)

// SWEAtlasPillar identifies one of the three SWE Atlas evaluation pillars.
type SWEAtlasPillar string

// Recognized SWE Atlas pillars.
const (
	PillarQnA         SWEAtlasPillar = "qna"
	PillarTestWriting SWEAtlasPillar = "test-writing"
	PillarRefactoring SWEAtlasPillar = "refactoring"
)

// SWEAtlas runs the Scale AI SWE Atlas suite. It splits tasks into three
// pillars — QnA, Test Writing, Refactoring — and reports a pillar-aware
// Insights summary so that the self-improvement loop can target the weakest
// pillar first.
type SWEAtlas struct {
	Loader   TaskLoader
	Executor TaskExecutor
}

// NewSWEAtlas constructs a SWEAtlas. nil values fall back to the built-in
// fixture loader and the StubExecutor.
func NewSWEAtlas(loader TaskLoader, executor TaskExecutor) *SWEAtlas {
	if loader == nil {
		loader = builtinLoader{suite: SuiteSWEAtlas, tasks: sweAtlasFixture()}
	}
	if executor == nil {
		executor = StubExecutor{}
	}
	return &SWEAtlas{Loader: loader, Executor: executor}
}

// Suite reports the benchmark suite identifier.
func (s *SWEAtlas) Suite() Suite { return SuiteSWEAtlas }

// Run executes the suite end-to-end.
func (s *SWEAtlas) Run(ctx context.Context, cfg RunConfig) (*Results, error) {
	return runTasks(ctx, runOptions{
		suite:    SuiteSWEAtlas,
		model:    cfg.Model,
		mode:     cfg.Mode,
		loader:   pickLoader(cfg.Loader, s.Loader),
		executor: s.Executor,
		limit:    cfg.Limit,
		budget:   cfg.BudgetUSD,
		tags:     cfg.Tags,
	})
}

// Analyze partitions tasks by pillar and reports per-pillar solve counts as
// FailurePatterns. The pillar with the lowest solve rate becomes the first
// hypothesis target.
func (s *SWEAtlas) Analyze(results *Results) (*Insights, error) {
	if results == nil {
		return nil, fmt.Errorf("benchmarks: Analyze requires non-nil results")
	}

	type pillarStats struct {
		total, solved int
		examples      []string
	}
	stats := map[string]*pillarStats{
		string(PillarQnA):         {},
		string(PillarTestWriting): {},
		string(PillarRefactoring): {},
	}

	for _, t := range results.Tasks {
		pillar := pillarOf(t)
		ps, ok := stats[pillar]
		if !ok {
			continue
		}
		ps.total++
		if t.Solved {
			ps.solved++
		} else if len(ps.examples) < 5 {
			ps.examples = append(ps.examples, t.TaskID)
		}
	}

	patterns := make([]FailurePattern, 0, len(stats))
	for name, ps := range stats {
		if ps.total == 0 {
			continue
		}
		patterns = append(patterns, FailurePattern{
			Name:       fmt.Sprintf("pillar:%s", name),
			Count:      ps.total - ps.solved,
			Examples:   ps.examples,
			Hypothesis: pillarHypothesis(name),
		})
	}
	sort.Slice(patterns, func(i, j int) bool { return patterns[i].Count > patterns[j].Count })

	return &Insights{
		Suite:    SuiteSWEAtlas,
		RunID:    results.RunID,
		Patterns: patterns,
	}, nil
}

// Compare diffs two SWE Atlas runs and returns the Delta.
func (s *SWEAtlas) Compare(baseline, test *Results) (*Delta, error) {
	return ComputeDelta(baseline, test)
}

func pillarOf(t TaskResult) string {
	if t.Metadata == nil {
		return ""
	}
	if v, ok := t.Metadata["pillar"].(string); ok {
		return v
	}
	return ""
}

func pillarHypothesis(pillar string) string {
	switch pillar {
	case string(PillarQnA):
		return "Improve answer extraction in DEAR Define phase: tasks fail when the agent paraphrases instead of citing"
	case string(PillarTestWriting):
		return "Strengthen Audit phase: agents write happy-path-only tests; require coverage of edge cases declared in acceptance-criteria"
	case string(PillarRefactoring):
		return "Add Reflect-phase functional-equivalence check: refactors regress when behavior shifts silently"
	}
	return "Pillar hypothesis unknown — extend pkg/benchmarks/sweatlas.go"
}
