package benchmarks

import (
	"context"
	"fmt"
	"time"
)

// StubExecutor is the default TaskExecutor used by suite scaffolds when no
// real executor is wired in. It records the task as errored with a clear
// message so that scaffold runs do not silently report misleading "solved"
// counts. Real executors will replace it.
type StubExecutor struct{}

// Execute satisfies TaskExecutor by returning a non-solved result that flags
// the task as not actually executed.
func (StubExecutor) Execute(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error) {
	_ = ctx
	return TaskResult{
		TaskID:   task.ID,
		Suite:    task.Suite,
		Mode:     mode,
		Solved:   false,
		Duration: 0,
		Error:    fmt.Sprintf("stub executor: task %q not executed (no TaskExecutor wired for mode %q model %q)", task.ID, mode, model),
	}, nil
}

// FuncExecutor adapts a plain function to the TaskExecutor interface, useful
// for tests and one-off integrations.
type FuncExecutor func(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error)

// Execute satisfies TaskExecutor by delegating to the underlying function.
func (f FuncExecutor) Execute(ctx context.Context, task TaskSpec, mode Mode, model string) (TaskResult, error) {
	return f(ctx, task, mode, model)
}

// MeasuredExecute wraps an executor call and overwrites the Duration field
// with the wall-clock time observed by the caller. Suite implementations use
// it so that wall time is recorded uniformly even if the underlying executor
// forgets to set Duration.
func MeasuredExecute(ctx context.Context, exec TaskExecutor, task TaskSpec, mode Mode, model string) (TaskResult, error) {
	start := time.Now()
	tr, err := exec.Execute(ctx, task, mode, model)
	if tr.Duration == 0 {
		tr.Duration = time.Since(start)
	}
	return tr, err
}
