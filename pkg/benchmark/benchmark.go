// Package benchmark provides a benchmark orchestration loop connecting
// task registration, variant execution, metric collection, statistical
// comparison, and automated decision-making.
package benchmark

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stats"
)

// Task defines a benchmark task to be executed.
type Task struct {
	Name        string
	Description string
	Fn          func(ctx context.Context, variant string) (*RunResult, error)
}

// RunResult holds the output of a single benchmark run.
type RunResult struct {
	Duration time.Duration
	CostUSD  float64
	Quality  float64 // 0-10
	Metadata map[string]any
}

// Metric identifies which metric to compare across variants.
type Metric string

const (
	MetricDuration Metric = "duration_ms"
	MetricCost     Metric = "cost_usd"
	MetricQuality  Metric = "quality"
)

// VariantResult stores results for a specific variant across multiple runs.
type VariantResult struct {
	Variant string
	Runs    []RunResult
}

// Comparison holds the statistical comparison between two variants for one metric.
type Comparison struct {
	Metric     Metric
	ControlVar string
	TestVar    string
	TTest      stats.TTestResult
	Effect     stats.EffectSizeResult
	CI         stats.ConfidenceInterval
	Significant bool
}

// Decision represents the automated decision about which variant is better.
type Decision struct {
	Winner     string // variant name or "inconclusive"
	Reason     string
	Comparisons []Comparison
}

// Report holds the complete benchmark report.
type Report struct {
	Task       string
	Variants   map[string]*VariantResult
	Decisions  []Decision
	StartTime  time.Time
	EndTime    time.Time
}

// TaskRegistry manages benchmark tasks.
type TaskRegistry struct {
	tasks map[string]Task
}

// NewTaskRegistry creates a new task registry.
func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{tasks: make(map[string]Task)}
}

// Register adds a task to the registry.
func (r *TaskRegistry) Register(task Task) {
	r.tasks[task.Name] = task
}

// Get returns a task by name.
func (r *TaskRegistry) Get(name string) (Task, bool) {
	t, ok := r.tasks[name]
	return t, ok
}

// List returns all registered task names.
func (r *TaskRegistry) List() []string {
	names := make([]string, 0, len(r.tasks))
	for name := range r.tasks {
		names = append(names, name)
	}
	return names
}

// VariantRunner executes a task for a given variant multiple times.
type VariantRunner struct{}

// NewVariantRunner creates a new variant runner.
func NewVariantRunner() *VariantRunner {
	return &VariantRunner{}
}

// Run executes a task for the given variant nRuns times.
func (vr *VariantRunner) Run(ctx context.Context, task Task, variant string, nRuns int) (*VariantResult, error) {
	result := &VariantResult{
		Variant: variant,
		Runs:    make([]RunResult, 0, nRuns),
	}

	for i := range nRuns {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		runResult, err := task.Fn(ctx, variant)
		if err != nil {
			return result, fmt.Errorf("run %d/%d for variant %q failed: %w", i+1, nRuns, variant, err)
		}
		result.Runs = append(result.Runs, *runResult)
	}

	return result, nil
}

// MetricCollector extracts metric values from variant results.
type MetricCollector struct{}

// NewMetricCollector creates a new metric collector.
func NewMetricCollector() *MetricCollector {
	return &MetricCollector{}
}

// Extract returns the values for a specific metric from variant results.
func (mc *MetricCollector) Extract(vr *VariantResult, metric Metric) []float64 {
	values := make([]float64, len(vr.Runs))
	for i, run := range vr.Runs {
		switch metric {
		case MetricDuration:
			values[i] = float64(run.Duration.Milliseconds())
		case MetricCost:
			values[i] = run.CostUSD
		case MetricQuality:
			values[i] = run.Quality
		}
	}
	return values
}

// StatComparator performs statistical comparisons between variants.
type StatComparator struct {
	alpha      float64
	nBootstrap int
}

// NewStatComparator creates a new stat comparator with default settings.
func NewStatComparator() *StatComparator {
	return &StatComparator{
		alpha:      0.05,
		nBootstrap: 1000,
	}
}

// Compare performs a statistical comparison between control and test for a metric.
func (sc *StatComparator) Compare(control, test stats.Sample, metric Metric, controlVar, testVar string) Comparison {
	ttest := stats.WelchTTest(control, test)
	effect := stats.EffectSize(control, test)
	ci := stats.BootstrapCI(control, test, sc.nBootstrap, 0.95)

	return Comparison{
		Metric:      metric,
		ControlVar:  controlVar,
		TestVar:     testVar,
		TTest:       ttest,
		Effect:      effect,
		CI:          ci,
		Significant: stats.IsSignificant(ttest.PValue, sc.alpha, effect),
	}
}

// DecisionEngine evaluates comparisons and decides which variant wins.
type DecisionEngine struct{}

// NewDecisionEngine creates a new decision engine.
func NewDecisionEngine() *DecisionEngine {
	return &DecisionEngine{}
}

// Decide evaluates comparisons and produces a decision.
// For cost/duration: lower is better. For quality: higher is better.
func (de *DecisionEngine) Decide(controlVar, testVar string, comparisons []Comparison) Decision {
	testWins := 0
	controlWins := 0
	sigCount := 0

	for _, c := range comparisons {
		if !c.Significant {
			continue
		}
		sigCount++

		// Determine which direction is "better"
		// For quality: higher is better (positive effect = control better)
		// For cost/duration: lower is better (negative effect = control better)
		switch c.Metric {
		case MetricQuality:
			if c.Effect.D > 0 {
				controlWins++
			} else {
				testWins++
			}
		case MetricCost, MetricDuration:
			if c.Effect.D < 0 {
				controlWins++
			} else {
				testWins++
			}
		}
	}

	if sigCount == 0 {
		return Decision{
			Winner:      "inconclusive",
			Reason:      "no statistically significant differences found",
			Comparisons: comparisons,
		}
	}

	if testWins > controlWins {
		return Decision{
			Winner:      testVar,
			Reason:      fmt.Sprintf("%s wins %d/%d significant comparisons", testVar, testWins, sigCount),
			Comparisons: comparisons,
		}
	}
	if controlWins > testWins {
		return Decision{
			Winner:      controlVar,
			Reason:      fmt.Sprintf("%s wins %d/%d significant comparisons", controlVar, controlWins, sigCount),
			Comparisons: comparisons,
		}
	}

	return Decision{
		Winner:      "inconclusive",
		Reason:      fmt.Sprintf("tied: each variant wins %d/%d significant comparisons", testWins, sigCount),
		Comparisons: comparisons,
	}
}

// Config configures the benchmark orchestrator.
type Config struct {
	Variants []string // Variant names to test (first is control)
	RunsPerVariant int
	Metrics  []Metric
}

// BenchmarkOrchestrator connects all components to run a complete benchmark.
type BenchmarkOrchestrator struct {
	registry   *TaskRegistry
	runner     *VariantRunner
	collector  *MetricCollector
	comparator *StatComparator
	engine     *DecisionEngine
}

// NewBenchmarkOrchestrator creates a fully-wired orchestrator.
func NewBenchmarkOrchestrator() *BenchmarkOrchestrator {
	return &BenchmarkOrchestrator{
		registry:   NewTaskRegistry(),
		runner:     NewVariantRunner(),
		collector:  NewMetricCollector(),
		comparator: NewStatComparator(),
		engine:     NewDecisionEngine(),
	}
}

// Registry returns the task registry for registering tasks.
func (bo *BenchmarkOrchestrator) Registry() *TaskRegistry {
	return bo.registry
}

// Run executes a benchmark for the named task with the given config.
func (bo *BenchmarkOrchestrator) Run(ctx context.Context, taskName string, cfg Config) (*Report, error) {
	task, ok := bo.registry.Get(taskName)
	if !ok {
		return nil, fmt.Errorf("task %q not found in registry", taskName)
	}

	if len(cfg.Variants) < 2 {
		return nil, fmt.Errorf("at least 2 variants required, got %d", len(cfg.Variants))
	}

	if cfg.RunsPerVariant <= 0 {
		cfg.RunsPerVariant = 5
	}
	if len(cfg.Metrics) == 0 {
		cfg.Metrics = []Metric{MetricDuration, MetricCost, MetricQuality}
	}

	report := &Report{
		Task:      taskName,
		Variants:  make(map[string]*VariantResult),
		StartTime: time.Now(),
	}

	// Run all variants
	for _, variant := range cfg.Variants {
		vr, err := bo.runner.Run(ctx, task, variant, cfg.RunsPerVariant)
		if err != nil {
			return nil, fmt.Errorf("variant %q: %w", variant, err)
		}
		report.Variants[variant] = vr
	}

	// Compare each variant against control (first variant)
	control := cfg.Variants[0]
	controlResult := report.Variants[control]

	for _, testVariant := range cfg.Variants[1:] {
		testResult := report.Variants[testVariant]
		var comparisons []Comparison

		for _, metric := range cfg.Metrics {
			controlValues := bo.collector.Extract(controlResult, metric)
			testValues := bo.collector.Extract(testResult, metric)

			controlSample := stats.NewSample(controlValues)
			testSample := stats.NewSample(testValues)

			comp := bo.comparator.Compare(controlSample, testSample, metric, control, testVariant)
			comparisons = append(comparisons, comp)
		}

		decision := bo.engine.Decide(control, testVariant, comparisons)
		report.Decisions = append(report.Decisions, decision)
	}

	report.EndTime = time.Now()
	return report, nil
}
