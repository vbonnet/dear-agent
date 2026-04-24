package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stats"
)

func TestTaskRegistry(t *testing.T) {
	r := NewTaskRegistry()
	task := Task{Name: "test-task", Description: "A test task"}
	r.Register(task)

	got, ok := r.Get("test-task")
	if !ok {
		t.Fatal("expected task to be found")
	}
	if got.Name != "test-task" {
		t.Errorf("Name = %q, want %q", got.Name, "test-task")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent task to not be found")
	}

	names := r.List()
	if len(names) != 1 || names[0] != "test-task" {
		t.Errorf("List = %v, want [test-task]", names)
	}
}

func TestVariantRunner(t *testing.T) {
	runner := NewVariantRunner()
	task := Task{
		Name: "simple",
		Fn: func(ctx context.Context, variant string) (*RunResult, error) {
			return &RunResult{
				Duration: 100 * time.Millisecond,
				CostUSD:  0.01,
				Quality:  8.0,
			}, nil
		},
	}

	result, err := runner.Run(context.Background(), task, "baseline", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Runs) != 3 {
		t.Errorf("Runs = %d, want 3", len(result.Runs))
	}
	if result.Variant != "baseline" {
		t.Errorf("Variant = %q, want %q", result.Variant, "baseline")
	}
}

func TestMetricCollector(t *testing.T) {
	mc := NewMetricCollector()
	vr := &VariantResult{
		Runs: []RunResult{
			{Duration: 100 * time.Millisecond, CostUSD: 0.01, Quality: 8.0},
			{Duration: 200 * time.Millisecond, CostUSD: 0.02, Quality: 9.0},
		},
	}

	durations := mc.Extract(vr, MetricDuration)
	if len(durations) != 2 || durations[0] != 100 || durations[1] != 200 {
		t.Errorf("Duration values = %v, want [100 200]", durations)
	}

	costs := mc.Extract(vr, MetricCost)
	if costs[0] != 0.01 || costs[1] != 0.02 {
		t.Errorf("Cost values = %v, want [0.01 0.02]", costs)
	}

	quality := mc.Extract(vr, MetricQuality)
	if quality[0] != 8.0 || quality[1] != 9.0 {
		t.Errorf("Quality values = %v, want [8.0 9.0]", quality)
	}
}

func TestDecisionEngine_Clear_Winner(t *testing.T) {
	de := NewDecisionEngine()

	comparisons := []Comparison{
		{
			Metric:      MetricCost,
			ControlVar:  "baseline",
			TestVar:     "engram",
			Significant: true,
			Effect:      stats.EffectSizeResult{D: 1.5, Interpretation: "large"}, // control costs more
		},
		{
			Metric:      MetricQuality,
			ControlVar:  "baseline",
			TestVar:     "engram",
			Significant: true,
			Effect:      stats.EffectSizeResult{D: -1.0, Interpretation: "large"}, // engram has higher quality
		},
	}

	decision := de.Decide("baseline", "engram", comparisons)
	if decision.Winner != "engram" {
		t.Errorf("Winner = %q, want %q", decision.Winner, "engram")
	}
}

func TestDecisionEngine_Inconclusive(t *testing.T) {
	de := NewDecisionEngine()

	comparisons := []Comparison{
		{Metric: MetricCost, Significant: false},
		{Metric: MetricQuality, Significant: false},
	}

	decision := de.Decide("baseline", "engram", comparisons)
	if decision.Winner != "inconclusive" {
		t.Errorf("Winner = %q, want %q", decision.Winner, "inconclusive")
	}
}

func TestBenchmarkOrchestrator_EndToEnd(t *testing.T) {
	orch := NewBenchmarkOrchestrator()

	runCount := 0
	orch.Registry().Register(Task{
		Name: "e2e-test",
		Fn: func(ctx context.Context, variant string) (*RunResult, error) {
			runCount++
			quality := 5.0
			cost := 0.10
			if variant == "engram" {
				quality = 9.0
				cost = 0.05
			}
			return &RunResult{
				Duration: 100 * time.Millisecond,
				CostUSD:  cost,
				Quality:  quality,
			}, nil
		},
	})

	cfg := Config{
		Variants:       []string{"baseline", "engram"},
		RunsPerVariant: 5,
		Metrics:        []Metric{MetricCost, MetricQuality},
	}

	report, err := orch.Run(context.Background(), "e2e-test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Task != "e2e-test" {
		t.Errorf("Task = %q, want %q", report.Task, "e2e-test")
	}
	if len(report.Variants) != 2 {
		t.Errorf("Variants = %d, want 2", len(report.Variants))
	}
	if len(report.Decisions) != 1 {
		t.Errorf("Decisions = %d, want 1", len(report.Decisions))
	}
	if runCount != 10 { // 5 runs * 2 variants
		t.Errorf("runCount = %d, want 10", runCount)
	}
}

func TestBenchmarkOrchestrator_TaskNotFound(t *testing.T) {
	orch := NewBenchmarkOrchestrator()
	_, err := orch.Run(context.Background(), "nonexistent", Config{Variants: []string{"a", "b"}})
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestBenchmarkOrchestrator_TooFewVariants(t *testing.T) {
	orch := NewBenchmarkOrchestrator()
	orch.Registry().Register(Task{Name: "test"})
	_, err := orch.Run(context.Background(), "test", Config{Variants: []string{"only-one"}})
	if err == nil {
		t.Error("expected error for < 2 variants")
	}
}

