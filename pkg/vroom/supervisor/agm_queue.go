package supervisor

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// AGMQueue is the production Queue adapter. The pending list lives in memory
// (populated by BeadsRoadmap.Accept → Enqueue, or by the caller seeding work
// before the mesh starts). Dispatch shells out to "agm session new" to spawn
// a real worker session for each dispatched task.
//
// The in-memory pending list does not survive process restarts. A persistent
// backing store is deferred to a follow-up.
type AGMQueue struct {
	mu      sync.Mutex
	pending []Task

	// AGMBin is the agm binary to invoke. Empty resolves "agm" on PATH.
	AGMBin string

	// Model is the Claude model workers spawn with. Defaults to "opus-200k".
	Model string

	// WorkerRole is the RBAC role passed to --role. Defaults to "worker".
	WorkerRole string

	// Timeout bounds a single "agm session new" invocation. Zero uses
	// defaultAGMDispatchTimeout.
	Timeout time.Duration

	// run is an injection seam for tests; nil uses exec.CommandContext.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

const defaultAGMDispatchTimeout = 60 * time.Second
const defaultDispatchModel = "opus-200k"
const defaultWorkerRole = "worker"

// Enqueue adds a task to the pending list. Returns an error on duplicate ID.
// This is called by BeadsRoadmap.Accept when Queue is wired.
func (q *AGMQueue) Enqueue(task Task) error {
	if task.ID == "" {
		return fmt.Errorf("AGMQueue.Enqueue: task ID required")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.pending {
		if t.ID == task.ID {
			return fmt.Errorf("AGMQueue.Enqueue: duplicate task ID %q", task.ID)
		}
	}
	if task.EnqueuedAt.IsZero() {
		task.EnqueuedAt = time.Now()
	}
	q.pending = append(q.pending, task)
	return nil
}

// Pending implements Queue. Returns a snapshot of tasks awaiting dispatch.
func (q *AGMQueue) Pending(_ context.Context) ([]Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Task, len(q.pending))
	copy(out, q.pending)
	return out, nil
}

// Dispatch implements Queue. It removes the task from the pending list and
// shells out to "agm session new" to spawn a worker session named
// "worker-<taskID>". The session runs detached (--detached) with the
// configured model and role.
func (q *AGMQueue) Dispatch(ctx context.Context, taskID, _ string) error {
	q.mu.Lock()
	found := false
	for i, t := range q.pending {
		if t.ID == taskID {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			found = true
			break
		}
	}
	q.mu.Unlock()

	if !found {
		return fmt.Errorf("AGMQueue.Dispatch: unknown task %q", taskID)
	}

	model := q.Model
	if model == "" {
		model = defaultDispatchModel
	}
	role := q.WorkerRole
	if role == "" {
		role = defaultWorkerRole
	}
	bin := q.AGMBin
	if bin == "" {
		bin = "agm"
	}

	timeout := q.Timeout
	if timeout <= 0 {
		timeout = defaultAGMDispatchTimeout
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := AGMDispatchArgs(taskID, model, role)

	if q.run != nil {
		_, err := q.run(dctx, bin, args...)
		return err
	}

	out, err := exec.CommandContext(dctx, bin, args...).CombinedOutput() //#nosec G204 -- bin is a fixed binary path; args are controlled caller values
	if err != nil {
		return fmt.Errorf("AGMQueue.Dispatch: agm session new worker-%s: %w\n%s",
			taskID, err, out)
	}
	return nil
}

// AGMDispatchArgs builds the `agm session new` argument list used to spawn a
// VROOM worker session.
func AGMDispatchArgs(taskID, model, role string) []string {
	return []string{
		"session", "new",
		"worker-" + taskID,
		"--model", model,
		"--role", role,
		"--mode", "auto",
		"--workspace", "oss",
		"--detached",
		"--prompt", fmt.Sprintf("Pick up bead %s and execute it.", taskID),
	}
}
