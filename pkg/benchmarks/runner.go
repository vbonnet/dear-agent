package benchmarks

import (
	"context"
	"fmt"
	"time"
)

// runOptions is the internal config used by runTasks. Suite implementations
// build it from their public RunConfig + their TaskLoader / TaskExecutor.
type runOptions struct {
	suite    Suite
	model    string
	mode     Mode
	loader   TaskLoader
	executor TaskExecutor
	limit    int
	budget   float64
	tags     map[string]string
}

// runTasks loads tasks, runs them serially through the executor, and stops
// early when the budget is exhausted or the context is cancelled. Concurrency
// is intentionally serial in this scaffold — adding concurrency means adding
// per-task locking around the cost accumulator and a worker pool, which we
// defer until a suite needs it.
func runTasks(ctx context.Context, opts runOptions) (*Results, error) {
	if opts.executor == nil {
		return nil, fmt.Errorf("benchmarks: TaskExecutor is required")
	}
	if opts.loader == nil {
		return nil, fmt.Errorf("benchmarks: TaskLoader is required")
	}
	if opts.mode == "" {
		return nil, fmt.Errorf("benchmarks: Mode is required")
	}

	tasks, err := opts.loader.Load(ctx, opts.suite, opts.limit)
	if err != nil {
		return nil, fmt.Errorf("load tasks: %w", err)
	}

	now := time.Now().UTC()
	results := &Results{
		Suite:     opts.suite,
		Mode:      opts.mode,
		Model:     opts.model,
		RunID:     fmt.Sprintf("%s-%s-%d", opts.suite, opts.mode, now.Unix()),
		StartedAt: now,
		Tasks:     make([]TaskResult, 0, len(tasks)),
	}

	var spent float64
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			results.Tasks = append(results.Tasks, TaskResult{
				TaskID: task.ID,
				Suite:  opts.suite,
				Mode:   opts.mode,
				Error:  err.Error(),
			})
			break
		}

		if opts.budget > 0 && spent >= opts.budget {
			results.Tasks = append(results.Tasks, TaskResult{
				TaskID: task.ID,
				Suite:  opts.suite,
				Mode:   opts.mode,
				Error:  fmt.Sprintf("budget exhausted: spent $%.4f >= cap $%.4f", spent, opts.budget),
			})
			continue
		}

		tr, execErr := opts.executor.Execute(ctx, task, opts.mode, opts.model)
		tr.TaskID = task.ID
		tr.Suite = opts.suite
		tr.Mode = opts.mode
		if execErr != nil && tr.Error == "" {
			tr.Error = execErr.Error()
		}
		spent += tr.CostUSD
		results.Tasks = append(results.Tasks, tr)
	}

	results.FinishedAt = time.Now().UTC()
	results.Summary = ComputeSummary(results.Tasks)
	return results, nil
}
