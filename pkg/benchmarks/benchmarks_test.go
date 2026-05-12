package benchmarks

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeSummary_EmptyHasInfiniteCostPerSolved(t *testing.T) {
	s := ComputeSummary(nil)
	if s.Total != 0 {
		t.Fatalf("Total = %d, want 0", s.Total)
	}
	if !math.IsInf(s.CostPerSolved, 1) {
		t.Fatalf("CostPerSolved = %v, want +Inf for empty input", s.CostPerSolved)
	}
}

func TestComputeSummary_CountsAndRates(t *testing.T) {
	tasks := []TaskResult{
		{TaskID: "a", Solved: true, CostUSD: 0.10, Duration: 1 * time.Second, TokensIn: 100, TokensOut: 50},
		{TaskID: "b", Solved: false, CostUSD: 0.05, Duration: 2 * time.Second, TokensIn: 200, TokensOut: 25},
		{TaskID: "c", Error: "boom", CostUSD: 0.02, Duration: 0},
	}
	s := ComputeSummary(tasks)
	if s.Total != 3 || s.Solved != 1 || s.Failed != 1 || s.Errored != 1 {
		t.Fatalf("counts = %+v, want 3/1/1/1", s)
	}
	if got := s.SolveRate; got != 1.0/3.0 {
		t.Fatalf("SolveRate = %v, want 1/3", got)
	}
	if got := s.TotalCostUSD; got < 0.169 || got > 0.171 {
		t.Fatalf("TotalCostUSD = %v, want ~0.17", got)
	}
	if got := s.CostPerSolved; got < 0.169 || got > 0.171 {
		t.Fatalf("CostPerSolved = %v, want ~0.17 (only one task solved)", got)
	}
	if s.TotalTokensIn != 300 || s.TotalTokensOut != 75 {
		t.Fatalf("token totals = %d/%d, want 300/75", s.TotalTokensIn, s.TotalTokensOut)
	}
}

func TestComputeDelta_ReportsImprovementsAndRegressions(t *testing.T) {
	baseline := &Results{
		Suite: SuiteSWEBenchLite,
		RunID: "baseline",
		Tasks: []TaskResult{
			{TaskID: "t1", Solved: true},
			{TaskID: "t2", Solved: true},
			{TaskID: "t3", Solved: false},
		},
	}
	baseline.Summary = ComputeSummary(baseline.Tasks)

	test := &Results{
		Suite: SuiteSWEBenchLite,
		RunID: "test",
		Tasks: []TaskResult{
			{TaskID: "t1", Solved: true},
			{TaskID: "t2", Solved: false},
			{TaskID: "t3", Solved: true},
		},
	}
	test.Summary = ComputeSummary(test.Tasks)

	d, err := ComputeDelta(baseline, test)
	if err != nil {
		t.Fatalf("ComputeDelta: %v", err)
	}
	if got, want := d.Improvements, []string{"t3"}; !equalStrings(got, want) {
		t.Fatalf("Improvements = %v, want %v", got, want)
	}
	if got, want := d.Regressions, []string{"t2"}; !equalStrings(got, want) {
		t.Fatalf("Regressions = %v, want %v", got, want)
	}
	if d.IsRegression {
		t.Fatalf("IsRegression = true, but solve rate is unchanged")
	}
}

func TestComputeDelta_RejectsMismatchedSuites(t *testing.T) {
	a := &Results{Suite: SuiteSWEBenchLite}
	b := &Results{Suite: SuiteVibeBench}
	if _, err := ComputeDelta(a, b); err == nil {
		t.Fatalf("expected error comparing different suites")
	}
}

func TestRegistry_RegisterListGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(NewSWEBenchLite(nil, nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(NewSWEBenchLite(nil, nil)); err == nil {
		t.Fatalf("expected duplicate-registration error")
	}
	suites := r.List()
	if len(suites) != 1 || suites[0] != SuiteSWEBenchLite {
		t.Fatalf("List = %v, want [%s]", suites, SuiteSWEBenchLite)
	}
	got, err := r.Get(SuiteSWEBenchLite)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Suite() != SuiteSWEBenchLite {
		t.Fatalf("Get returned suite %q", got.Suite())
	}
	if _, err := r.Get(SuiteVibeBench); err == nil {
		t.Fatalf("expected error for unregistered suite")
	}
}

func TestDefaultRegistryHasAllSuites(t *testing.T) {
	want := map[Suite]bool{
		SuiteSWEBenchLite:     true,
		SuiteSWEBenchVerified: true,
		SuiteSWEAtlas:         true,
		SuiteVibeBench:        true,
	}
	for _, s := range DefaultRegistry.List() {
		delete(want, s)
	}
	if len(want) != 0 {
		t.Fatalf("DefaultRegistry missing suites: %v", want)
	}
}

func TestRun_AppliesBudgetCap(t *testing.T) {
	exec := FuncExecutor(func(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error) {
		return TaskResult{Solved: true, CostUSD: 0.10}, nil
	})
	bench := NewSWEBenchLite(nil, exec)
	results, err := bench.Run(context.Background(), RunConfig{
		Mode:      ModeRaw,
		Model:     "test-model",
		BudgetUSD: 0.15, // enough for 1 task at $0.10, the second pushes spend to $0.20 > cap, third gets blocked
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(results.Tasks); got != 3 {
		t.Fatalf("Tasks = %d, want 3 (budget caps third task only)", got)
	}
	// First two tasks run, third is budget-exhausted.
	if results.Tasks[0].Error != "" || results.Tasks[1].Error != "" {
		t.Fatalf("first two tasks should run; got errors: %q / %q", results.Tasks[0].Error, results.Tasks[1].Error)
	}
	if results.Tasks[2].Error == "" {
		t.Fatalf("third task should be budget-exhausted; got no error")
	}
}

func TestRun_StubExecutorMarksTasksAsErrored(t *testing.T) {
	bench := NewSWEBenchLite(nil, nil) // nil executor → StubExecutor
	results, err := bench.Run(context.Background(), RunConfig{Mode: ModeRaw, Model: "x"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results.Summary.Solved != 0 {
		t.Fatalf("Solved = %d, want 0 (stub must not lie about success)", results.Summary.Solved)
	}
	if results.Summary.Errored != results.Summary.Total {
		t.Fatalf("Errored = %d, want %d (every stub task must be flagged)", results.Summary.Errored, results.Summary.Total)
	}
}

// fakeLoader is a TaskLoader stub that returns the configured task list.
type fakeLoader struct{ tasks []TaskSpec }

func (l fakeLoader) Load(_ context.Context, _ Suite, limit int) ([]TaskSpec, error) {
	out := l.tasks
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func TestRun_CfgLoaderOverridesDefault(t *testing.T) {
	// The suite is registered with the built-in fixture (3 tasks); the
	// per-run cfg.Loader should win and feed the executor exactly one task.
	exec := FuncExecutor(func(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error) {
		return TaskResult{Solved: true}, nil
	})
	bench := NewSWEBenchLite(nil, exec)

	cfg := RunConfig{
		Mode:   ModeRaw,
		Model:  "x",
		Loader: fakeLoader{tasks: []TaskSpec{{ID: "override-only", Suite: SuiteSWEBenchLite}}},
	}
	results, err := bench.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(results.Tasks); got != 1 {
		t.Fatalf("Tasks = %d, want 1 (override loader had one task)", got)
	}
	if results.Tasks[0].TaskID != "override-only" {
		t.Fatalf("TaskID = %q, want override-only", results.Tasks[0].TaskID)
	}
}

func TestRun_PropagatesContextCancellation(t *testing.T) {
	called := 0
	exec := FuncExecutor(func(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error) {
		called++
		return TaskResult{Solved: true}, nil
	})
	bench := NewSWEBenchLite(nil, exec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := bench.Run(ctx, RunConfig{Mode: ModeRaw, Model: "x"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called != 0 {
		t.Fatalf("executor should not be called when context is cancelled before any task; called=%d", called)
	}
	if len(results.Tasks) == 0 || results.Tasks[0].Error == "" {
		t.Fatalf("expected first task to be marked with context error")
	}
}

func TestPersist_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := &Results{
		Suite: SuiteSWEBenchLite,
		Mode:  ModeDearAgent,
		Model: "claude-opus-4-7",
		RunID: "round-trip-1",
		Tasks: []TaskResult{{TaskID: "t1", Solved: true, CostUSD: 0.05}},
	}
	r.Summary = ComputeSummary(r.Tasks)

	path, err := WriteResults(r, dir)
	if err != nil {
		t.Fatalf("WriteResults: %v", err)
	}
	if filepath.Base(path) != "round-trip-1.json" {
		t.Fatalf("unexpected path %q", path)
	}
	got, err := ReadResults(path)
	if err != nil {
		t.Fatalf("ReadResults: %v", err)
	}
	if got.RunID != r.RunID || got.Summary.Solved != 1 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestSWEAtlas_AnalyzeReportsPerPillarPatterns(t *testing.T) {
	results := &Results{
		Suite: SuiteSWEAtlas,
		RunID: "atlas-1",
		Tasks: []TaskResult{
			{TaskID: "qna-1", Solved: true, Metadata: map[string]any{"pillar": string(PillarQnA)}},
			{TaskID: "qna-2", Solved: false, Metadata: map[string]any{"pillar": string(PillarQnA)}},
			{TaskID: "test-1", Solved: false, Metadata: map[string]any{"pillar": string(PillarTestWriting)}},
			{TaskID: "test-2", Solved: false, Metadata: map[string]any{"pillar": string(PillarTestWriting)}},
			{TaskID: "ref-1", Solved: true, Metadata: map[string]any{"pillar": string(PillarRefactoring)}},
		},
	}
	atlas := NewSWEAtlas(nil, nil)
	insights, err := atlas.Analyze(results)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(insights.Patterns) == 0 {
		t.Fatalf("expected per-pillar patterns")
	}
	// Test-writing pillar should be the worst: 2/2 unsolved.
	if insights.Patterns[0].Name != "pillar:test-writing" {
		t.Fatalf("first pattern = %q, want pillar:test-writing", insights.Patterns[0].Name)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
