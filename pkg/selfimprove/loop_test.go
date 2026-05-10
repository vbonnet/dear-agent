package selfimprove

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

// fakeBench is a Benchmark stub that returns canned Results per call. It
// drives the loop without touching any real benchmark infrastructure.
type fakeBench struct {
	suite     benchmarks.Suite
	calls     int
	scripted  []*benchmarks.Results
	analyze   *benchmarks.Insights
	analyzeErr error
}

func (f *fakeBench) Suite() benchmarks.Suite { return f.suite }

func (f *fakeBench) Run(ctx context.Context, cfg benchmarks.RunConfig) (*benchmarks.Results, error) {
	idx := f.calls
	f.calls++
	if idx >= len(f.scripted) {
		idx = len(f.scripted) - 1
	}
	return f.scripted[idx], nil
}

func (f *fakeBench) Analyze(_ *benchmarks.Results) (*benchmarks.Insights, error) {
	if f.analyzeErr != nil {
		return nil, f.analyzeErr
	}
	return f.analyze, nil
}

func (f *fakeBench) Compare(baseline, test *benchmarks.Results) (*benchmarks.Delta, error) {
	return benchmarks.ComputeDelta(baseline, test)
}

type fakeProposer struct {
	hypotheses []Hypothesis
}

func (p *fakeProposer) Propose(_ context.Context, _ *benchmarks.Insights, _ string) ([]Hypothesis, error) {
	return p.hypotheses, nil
}

type fakeApplier struct {
	mu       sync.Mutex
	applied  []Patch
	reverted []Patch
	failApply bool
}

func (a *fakeApplier) Apply(_ context.Context, h Hypothesis) (Patch, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failApply {
		return Patch{}, errors.New("apply blew up")
	}
	p := Patch{Hypothesis: h, Diff: "+ // " + h.Description}
	a.applied = append(a.applied, p)
	return p, nil
}

func (a *fakeApplier) Revert(_ context.Context, p Patch) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reverted = append(a.reverted, p)
	return nil
}

func resultsWith(suite benchmarks.Suite, runID string, taskIDs []string, solved []bool) *benchmarks.Results {
	tasks := make([]benchmarks.TaskResult, len(taskIDs))
	for i, id := range taskIDs {
		tasks[i] = benchmarks.TaskResult{TaskID: id, Solved: solved[i], CostUSD: 0.01}
	}
	r := &benchmarks.Results{Suite: suite, RunID: runID, Tasks: tasks}
	r.Summary = benchmarks.ComputeSummary(tasks)
	return r
}

func TestLoop_AcceptsImprovement(t *testing.T) {
	suite := benchmarks.SuiteSWEBenchLite
	baseline := resultsWith(suite, "baseline", []string{"a", "b", "c"}, []bool{true, false, false})
	improved := resultsWith(suite, "post", []string{"a", "b", "c"}, []bool{true, true, false})

	bench := &fakeBench{
		suite:    suite,
		scripted: []*benchmarks.Results{baseline, improved},
		analyze:  &benchmarks.Insights{Suite: suite, RunID: "baseline"},
	}
	proposer := &fakeProposer{hypotheses: []Hypothesis{{ID: "h1", Source: "claude", Description: "fix b"}}}
	applier := &fakeApplier{}

	loop := New(bench, proposer, applier, nil)
	results, err := loop.Run(context.Background(), LoopConfig{
		Suite: suite, Mode: benchmarks.ModeDearAgent, Model: "claude",
		MaxCycles: 1, RegressionGate: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Accepted {
		t.Fatalf("cycle should be accepted; reason: %s", results[0].Reason)
	}
	if len(applier.reverted) != 0 {
		t.Fatalf("accepted cycle should not revert any patches, got %d reverts", len(applier.reverted))
	}
}

func TestLoop_RegressionGateRevertsAndStops(t *testing.T) {
	suite := benchmarks.SuiteSWEBenchLite
	baseline := resultsWith(suite, "baseline", []string{"a", "b"}, []bool{true, true})
	worse := resultsWith(suite, "post", []string{"a", "b"}, []bool{true, false})

	bench := &fakeBench{
		suite:    suite,
		scripted: []*benchmarks.Results{baseline, worse},
		analyze:  &benchmarks.Insights{Suite: suite},
	}
	proposer := &fakeProposer{hypotheses: []Hypothesis{{ID: "bad", Source: "gpt", Description: "regress b"}}}
	applier := &fakeApplier{}

	loop := New(bench, proposer, applier, nil)
	results, err := loop.Run(context.Background(), LoopConfig{
		Suite: suite, Mode: benchmarks.ModeDearAgent, Model: "claude",
		MaxCycles: 3, RegressionGate: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("regression gate should stop after first reject; got %d cycles", len(results))
	}
	if results[0].Accepted {
		t.Fatalf("cycle must not be accepted on regression")
	}
	if len(applier.reverted) != len(applier.applied) {
		t.Fatalf("all applied patches must be reverted on rejection: applied=%d reverted=%d",
			len(applier.applied), len(applier.reverted))
	}
}

func TestLoop_NoHypothesesEndsCycle(t *testing.T) {
	suite := benchmarks.SuiteSWEBenchLite
	baseline := resultsWith(suite, "baseline", []string{"a"}, []bool{false})
	bench := &fakeBench{
		suite:    suite,
		scripted: []*benchmarks.Results{baseline},
		analyze:  &benchmarks.Insights{Suite: suite},
	}
	loop := New(bench, &fakeProposer{}, &fakeApplier{}, nil)
	results, err := loop.Run(context.Background(), LoopConfig{
		Suite: suite, Mode: benchmarks.ModeDearAgent, Model: "claude", MaxCycles: 2,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 || results[0].Reason != "no hypotheses proposed" {
		t.Fatalf("expected single cycle with no-hypotheses reason; got %+v", results)
	}
}

func TestLoop_ValidatesSuiteMatch(t *testing.T) {
	bench := &fakeBench{suite: benchmarks.SuiteVibeBench}
	loop := New(bench, &fakeProposer{}, &fakeApplier{}, nil)
	_, err := loop.Run(context.Background(), LoopConfig{
		Suite: benchmarks.SuiteSWEBenchLite, Mode: benchmarks.ModeRaw, Model: "x",
	})
	if err == nil {
		t.Fatalf("expected suite-mismatch error")
	}
}

func TestLoop_BudgetCapStopsLoop(t *testing.T) {
	suite := benchmarks.SuiteSWEBenchLite
	// baseline costs $0.03 (3 tasks @ $0.01); post costs another $0.03; budget is $0.05
	baseline := resultsWith(suite, "baseline", []string{"a", "b", "c"}, []bool{true, false, false})
	post := resultsWith(suite, "post", []string{"a", "b", "c"}, []bool{true, true, false})

	bench := &fakeBench{
		suite:    suite,
		scripted: []*benchmarks.Results{baseline, post},
		analyze:  &benchmarks.Insights{Suite: suite},
	}
	proposer := &fakeProposer{hypotheses: []Hypothesis{{ID: "h", Source: "claude", Description: "x"}}}
	loop := New(bench, proposer, &fakeApplier{}, nil)
	results, err := loop.Run(context.Background(), LoopConfig{
		Suite: suite, Mode: benchmarks.ModeDearAgent, Model: "claude",
		MaxCycles: 5, BudgetUSD: 0.05, RegressionGate: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// baseline cost $0.03 < $0.05, so cycle 1 runs (post adds $0.03 → total $0.06).
	// Cycle 2 must short-circuit on budget.
	if len(results) < 2 {
		t.Fatalf("expected at least 2 cycles (one ran, one budget-stopped); got %d", len(results))
	}
	last := results[len(results)-1]
	if last.Reason == "" || !contains(last.Reason, "budget") {
		t.Fatalf("last cycle should mention budget; got reason=%q", last.Reason)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
