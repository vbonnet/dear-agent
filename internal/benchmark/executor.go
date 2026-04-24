package benchmark

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/vbonnet/dear-agent/internal/common"
)

// Executor runs benchmarks on a command
type Executor struct {
	Command    string        // Command to execute
	Runs       int           // Number of measured runs
	WarmupRuns int           // Number of warmup runs
	Scenario   string        // Test scenario (empty, small, medium)
	Timeout    time.Duration // Timeout per run (default: 10s)
}

// NewExecutor creates a new benchmark executor with default values
func NewExecutor(command string) *Executor {
	return &Executor{
		Command:    command,
		Runs:       10,
		WarmupRuns: 1,
		Scenario:   "small",
		Timeout:    10 * time.Second,
	}
}

// Run executes the benchmark and returns results
func (e *Executor) Run() (*BenchmarkResult, error) {
	result := &BenchmarkResult{
		Command:    e.Command,
		Scenario:   e.Scenario,
		Runs:       e.Runs,
		WarmupRuns: e.WarmupRuns,
		Timings:    make([]time.Duration, 0, e.Runs),
		Errors:     []string{},
	}

	// Stage test files for scenario
	if err := common.StageTestFiles(e.Scenario); err != nil {
		return nil, fmt.Errorf("failed to stage test files: %w", err)
	}
	defer common.UnstageTestFiles()

	// Run warmup iterations
	for i := 0; i < e.WarmupRuns; i++ {
		_, err := e.runOnce()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("warmup run %d: %v", i+1, err))
		}
	}

	// Run measured iterations
	for i := 0; i < e.Runs; i++ {
		duration, err := e.runOnce()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("run %d: %v", i+1, err))
			continue
		}
		result.Timings = append(result.Timings, duration)
	}

	// Calculate statistics only if we have successful runs
	if len(result.Timings) > 0 {
		if err := calculateStats(result); err != nil {
			return nil, fmt.Errorf("failed to calculate statistics: %w", err)
		}
	}

	return result, nil
}

// runOnce executes the command once and returns the duration
func (e *Executor) runOnce() (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", e.Command)
	// Discard stdout/stderr for accurate timing
	cmd.Stdout = nil
	cmd.Stderr = nil

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("timeout after %v", e.Timeout)
	}

	if err != nil {
		return elapsed, fmt.Errorf("command failed: %w", err)
	}

	return elapsed, nil
}
