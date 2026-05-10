package benchmarks

import (
	"context"
	"time"
)

// Mode is the agent execution mode for a benchmark run. The same Suite can be
// driven in either Mode so that A/B comparison isolates the effect of the
// DEAR protocol from the underlying model.
type Mode string

// Recognized Mode values.
const (
	// ModeDearAgent runs each task with the full DEAR protocol — Define,
	// Execute, Audit, Reflect — enforced by the dear-agent harness.
	ModeDearAgent Mode = "dear-agent"

	// ModeRaw runs each task with the same model directly behind a thin
	// harness (no DEAR enforcement, no audit phase, no reflection). It is
	// the baseline against which dear-agent must prove value.
	ModeRaw Mode = "raw"
)

// Suite identifies a coding benchmark suite. New suites are added to the
// registry by calling Register.
type Suite string

// Recognized Suite values.
const (
	SuiteSWEBenchLite     Suite = "swe-bench-lite"
	SuiteSWEBenchVerified Suite = "swe-bench-verified"
	SuiteSWEAtlas         Suite = "swe-atlas"
	SuiteVibeBench        Suite = "vibe-bench"
)

// TaskSpec is the input handed to a TaskExecutor for a single benchmark task.
type TaskSpec struct {
	ID         string         `json:"id"`
	Suite      Suite          `json:"suite"`
	Repo       string         `json:"repo,omitempty"`
	Prompt     string         `json:"prompt"`
	BaseCommit string         `json:"base_commit,omitempty"`
	TestPatch  string         `json:"test_patch,omitempty"`
	Pillar     string         `json:"pillar,omitempty"` // SWE-Atlas: qna|test-writing|refactoring
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// TaskResult is the outcome of running a single benchmark task.
type TaskResult struct {
	TaskID     string         `json:"task_id"`
	Suite      Suite          `json:"suite"`
	Mode       Mode           `json:"mode"`
	Solved     bool           `json:"solved"`
	Duration   time.Duration  `json:"duration_ns"`
	TokensIn   int            `json:"tokens_in"`
	TokensOut  int            `json:"tokens_out"`
	CostUSD    float64        `json:"cost_usd"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// RunConfig configures a single benchmark run.
type RunConfig struct {
	Mode        Mode    `json:"mode"`
	Model       string  `json:"model"`
	Limit       int     `json:"limit,omitempty"`        // 0 = run all
	BudgetUSD   float64 `json:"budget_usd,omitempty"`   // 0 = unbounded
	ResultsDir  string  `json:"results_dir,omitempty"`
	Concurrency int     `json:"concurrency,omitempty"`  // 0 = serial
	Seed        int64   `json:"seed,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// Summary aggregates Results metrics for at-a-glance reporting.
//
// CostPerSolved can be +Inf when no tasks solved; the Summary's MarshalJSON
// encodes it as JSON null in that case so callers do not have to special-case
// non-finite floats.
type Summary struct {
	Total          int           `json:"total"`
	Solved         int           `json:"solved"`
	Failed         int           `json:"failed"`
	Errored        int           `json:"errored"`
	SolveRate      float64       `json:"solve_rate"`     // Solved / Total
	TotalCostUSD   float64       `json:"total_cost_usd"`
	CostPerSolved  float64       `json:"-"` // first-class cost-per-intelligence metric; serialized via MarshalJSON
	TotalTokensIn  int           `json:"total_tokens_in"`
	TotalTokensOut int           `json:"total_tokens_out"`
	AvgDuration    time.Duration `json:"avg_duration_ns"`
}

// Results captures the full outcome of one benchmark run.
type Results struct {
	Suite      Suite        `json:"suite"`
	Mode       Mode         `json:"mode"`
	Model      string       `json:"model"`
	RunID      string       `json:"run_id"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Tasks      []TaskResult `json:"tasks"`
	Summary    Summary      `json:"summary"`
}

// FailurePattern is a recurring failure mode discovered by Analyze. The
// Hypothesis field is the proposer-friendly seed that pkg/selfimprove turns
// into concrete improvement attempts.
type FailurePattern struct {
	Name       string   `json:"name"`
	Count      int      `json:"count"`
	Examples   []string `json:"examples"`
	Hypothesis string   `json:"hypothesis"`
}

// Insights is the output of Benchmark.Analyze.
type Insights struct {
	Suite     Suite            `json:"suite"`
	RunID     string           `json:"run_id"`
	Patterns  []FailurePattern `json:"patterns"`
	TopErrors []string         `json:"top_errors,omitempty"`
	Notes     []string         `json:"notes,omitempty"`
}

// Delta is the result of comparing two runs of the same suite. Sign convention:
// positive SolveRateDelta means Test improved over Baseline.
type Delta struct {
	Suite              Suite    `json:"suite"`
	BaselineRunID      string   `json:"baseline_run_id"`
	TestRunID          string   `json:"test_run_id"`
	SolveRateDelta     float64  `json:"solve_rate_delta"`
	CostUSDDelta       float64  `json:"cost_usd_delta"`
	CostPerSolvedDelta float64  `json:"cost_per_solved_delta"`
	Regressions        []string `json:"regressions,omitempty"`  // task IDs solved in baseline but not test
	Improvements       []string `json:"improvements,omitempty"` // task IDs solved in test but not baseline
	IsRegression       bool     `json:"is_regression"`
}

// Benchmark is a coding-benchmark suite implementation.
//
// Run executes the suite with the given config and returns Results. Analyze
// inspects Results to surface failure patterns. Compare diffs two runs and
// returns a Delta. The contract is intentionally narrow so that a self-
// improvement loop can drive any suite uniformly.
type Benchmark interface {
	Suite() Suite
	Run(ctx context.Context, cfg RunConfig) (*Results, error)
	Analyze(results *Results) (*Insights, error)
	Compare(baseline, test *Results) (*Delta, error)
}

// TaskExecutor runs a single benchmark task. Suite implementations delegate
// to a TaskExecutor so that the dear-agent vs. raw split is a swap of executor
// rather than a fork of the suite. Implementations must be safe for concurrent
// use.
type TaskExecutor interface {
	Execute(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error)
}

// TaskLoader returns the tasks for a Suite. Implementations may load from
// disk, from a remote dataset, or return a small built-in fixture for
// scaffolding and tests.
type TaskLoader interface {
	Load(ctx context.Context, suite Suite, limit int) ([]TaskSpec, error)
}
