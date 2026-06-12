package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOrchestrator_Role(t *testing.T) {
	trail, _ := newBufferTrail()
	o, err := NewOrchestrator(trail, NewInMemoryQueue())
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	if o.Role() != RoleOrchestrator {
		t.Errorf("Role = %q, want %q", o.Role(), RoleOrchestrator)
	}
}

func TestNewOrchestrator_RejectsNilDeps(t *testing.T) {
	trail, _ := newBufferTrail()
	if _, err := NewOrchestrator(nil, NewInMemoryQueue()); err == nil {
		t.Error("nil trail accepted")
	}
	if _, err := NewOrchestrator(trail, nil); err == nil {
		t.Error("nil queue accepted")
	}
}

func TestOrchestrator_Tick_DispatchesAllPending(t *testing.T) {
	q := NewInMemoryQueue()
	must(t, q.Enqueue(Task{ID: "t1", Title: "fix lint", Worker: "coder"}))
	must(t, q.Enqueue(Task{ID: "t2", Title: "review PR", Worker: "code-reviewer"}))

	trail, buf := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := q.Dispatched(); !equalSorted(got, []string{"t1", "t2"}) {
		t.Errorf("Dispatched = %v, want [t1 t2]", got)
	}
	// Trail has two dispatched records.
	count := 0
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.orch.dispatched" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("dispatched-record count = %d, want 2", count)
	}
}

func TestOrchestrator_Tick_PendingErrorPropagates(t *testing.T) {
	q := &errorQueue{err: errors.New("boom")}
	trail, _ := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	if err := o.Tick(context.Background()); err == nil {
		t.Error("Tick returned nil when Pending errored")
	}
}

func TestOrchestrator_Tick_DispatchErrorRecordedButContinues(t *testing.T) {
	q := &flakyQueue{
		InMemoryQueue: NewInMemoryQueue(),
		failDispatch:  map[string]bool{"t1": true},
	}
	must(t, q.Enqueue(Task{ID: "t1", Title: "x", Worker: "coder"}))
	must(t, q.Enqueue(Task{ID: "t2", Title: "y", Worker: "coder"}))

	trail, buf := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick returned err = %v; single-task failures must not abort the tick", err)
	}
	if got := q.Dispatched(); len(got) != 1 || got[0] != "t2" {
		t.Errorf("Dispatched = %v, want [t2]", got)
	}
	saw := false
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.orch.dispatch_failed" {
			saw = true
		}
	}
	if !saw {
		t.Error("no dispatch_failed record in trail")
	}
}

func TestOrchestrator_Tick_StaleContextEscalation(t *testing.T) {
	// A task that can't be dispatched (flakyQueue) and was enqueued 25h ago
	// must trigger supervisor.orch.stale_context_escalation.
	q := &flakyQueue{
		InMemoryQueue: NewInMemoryQueue(),
		failDispatch:  map[string]bool{"t-stale": true},
	}
	old := time.Now().Add(-25 * time.Hour)
	must(t, q.Enqueue(Task{ID: "t-stale", Title: "waiting for design", Worker: "coder", EnqueuedAt: old}))

	trail, buf := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	// Fix the clock at "now" so the age calculation is deterministic.
	now := time.Now()
	o.now = func() time.Time { return now }
	o.staleThreshold = 24 * time.Hour

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	saw := false
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.orch.stale_context_escalation" {
			saw = true
		}
	}
	if !saw {
		t.Error("no supervisor.orch.stale_context_escalation in trail for 25h-old undispatched task")
	}
}

func TestOrchestrator_Tick_FreshTaskNoEscalation(t *testing.T) {
	// A task enqueued 1h ago must NOT trigger stale escalation.
	q := &flakyQueue{
		InMemoryQueue: NewInMemoryQueue(),
		failDispatch:  map[string]bool{"t-fresh": true},
	}
	recent := time.Now().Add(-1 * time.Hour)
	must(t, q.Enqueue(Task{ID: "t-fresh", Title: "just started", Worker: "coder", EnqueuedAt: recent}))

	trail, buf := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	now := time.Now()
	o.now = func() time.Time { return now }
	o.staleThreshold = 24 * time.Hour

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.orch.stale_context_escalation" {
			t.Error("unexpected stale_context_escalation for 1h-old task")
		}
	}
}

func TestOrchestrator_Tick_DispatchedTaskNotEscalated(t *testing.T) {
	// A task enqueued 48h ago that IS successfully dispatched must NOT trigger
	// stale escalation — it is no longer blocked.
	q := NewInMemoryQueue()
	old := time.Now().Add(-48 * time.Hour)
	must(t, q.Enqueue(Task{ID: "t-old-ok", Title: "ancient but dispatched", Worker: "coder", EnqueuedAt: old}))

	trail, buf := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	now := time.Now()
	o.now = func() time.Time { return now }
	o.staleThreshold = 24 * time.Hour

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.orch.stale_context_escalation" {
			t.Error("stale_context_escalation fired for a successfully dispatched task")
		}
	}
}

type errorQueue struct {
	Queue
	err error
}

func (e *errorQueue) Pending(context.Context) ([]Task, error)             { return nil, e.err }
func (e *errorQueue) Dispatch(context.Context, string, string) error      { return nil }

type flakyQueue struct {
	*InMemoryQueue
	failDispatch map[string]bool
}

func (f *flakyQueue) Dispatch(ctx context.Context, taskID, worker string) error {
	if f.failDispatch[taskID] {
		return errors.New("simulated dispatch failure")
	}
	return f.InMemoryQueue.Dispatch(ctx, taskID, worker)
}
