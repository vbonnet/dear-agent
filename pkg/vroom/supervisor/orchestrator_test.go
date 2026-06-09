package supervisor

import (
	"context"
	"errors"
	"testing"
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
