package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// In-memory implementations of Roadmap / Queue / ResourceProbe used by:
//   - the cmd/vroom-mesh "try it" runner;
//   - supervisor tests that don't want to stand up real adapters.
//
// These are deliberately NOT exposed as production substrates — they exist
// to let the mesh be exercised end-to-end before the real adapters land.

// InMemoryRoadmap is a thread-safe in-memory Roadmap.
type InMemoryRoadmap struct {
	mu       sync.Mutex
	pending  map[string]WorkProposal
	accepted []string
	rejected []rejectedProposal
}

type rejectedProposal struct {
	ID     string
	Reason string
}

// NewInMemoryRoadmap returns an empty roadmap.
func NewInMemoryRoadmap() *InMemoryRoadmap {
	return &InMemoryRoadmap{pending: map[string]WorkProposal{}}
}

// Submit adds a proposal to the pending set. Returns an error on duplicate ID.
func (r *InMemoryRoadmap) Submit(p WorkProposal) error {
	if p.ID == "" {
		return errors.New("InMemoryRoadmap: WorkProposal.ID required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.pending[p.ID]; dup {
		return fmt.Errorf("InMemoryRoadmap: duplicate proposal ID %q", p.ID)
	}
	r.pending[p.ID] = p
	return nil
}

// PendingProposals implements Roadmap. Returns proposals in ID-sorted
// order for deterministic iteration.
func (r *InMemoryRoadmap) PendingProposals(_ context.Context) ([]WorkProposal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.pending))
	for id := range r.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]WorkProposal, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.pending[id])
	}
	return out, nil
}

// Accept implements Roadmap.
func (r *InMemoryRoadmap) Accept(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[id]; !ok {
		return fmt.Errorf("InMemoryRoadmap: unknown proposal %q", id)
	}
	delete(r.pending, id)
	r.accepted = append(r.accepted, id)
	return nil
}

// Reject implements Roadmap.
func (r *InMemoryRoadmap) Reject(_ context.Context, id, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[id]; !ok {
		return fmt.Errorf("InMemoryRoadmap: unknown proposal %q", id)
	}
	delete(r.pending, id)
	r.rejected = append(r.rejected, rejectedProposal{ID: id, Reason: reason})
	return nil
}

// Accepted returns the IDs of accepted proposals in acceptance order.
// Useful for tests.
func (r *InMemoryRoadmap) Accepted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.accepted...)
}

// Rejected returns the IDs of rejected proposals in rejection order.
func (r *InMemoryRoadmap) Rejected() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.rejected))
	for _, rp := range r.rejected {
		out = append(out, rp.ID)
	}
	return out
}

// InMemoryQueue is a thread-safe in-memory Queue.
type InMemoryQueue struct {
	mu         sync.Mutex
	pending    map[string]Task
	dispatched []dispatchedTask
}

type dispatchedTask struct {
	ID     string
	Worker string
}

// NewInMemoryQueue returns an empty queue.
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{pending: map[string]Task{}}
}

// Enqueue adds a task to the pending set. Returns an error on duplicate ID.
// If t.EnqueuedAt is zero it is set to the current time so the Orchestrator
// can detect stale tasks (ce-6as.3). Callers may set EnqueuedAt explicitly
// to control apparent age (useful in tests).
func (q *InMemoryQueue) Enqueue(t Task) error {
	if t.ID == "" {
		return errors.New("InMemoryQueue: Task.ID required")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, dup := q.pending[t.ID]; dup {
		return fmt.Errorf("InMemoryQueue: duplicate task ID %q", t.ID)
	}
	if t.EnqueuedAt.IsZero() {
		t.EnqueuedAt = time.Now()
	}
	q.pending[t.ID] = t
	return nil
}

// Pending implements Queue. Returns tasks in ID-sorted order.
func (q *InMemoryQueue) Pending(_ context.Context) ([]Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := make([]string, 0, len(q.pending))
	for id := range q.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Task, 0, len(ids))
	for _, id := range ids {
		out = append(out, q.pending[id])
	}
	return out, nil
}

// Dispatch implements Queue.
func (q *InMemoryQueue) Dispatch(_ context.Context, taskID, worker string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.pending[taskID]; !ok {
		return fmt.Errorf("InMemoryQueue: unknown task %q", taskID)
	}
	delete(q.pending, taskID)
	q.dispatched = append(q.dispatched, dispatchedTask{ID: taskID, Worker: worker})
	return nil
}

// Dispatched returns the IDs of dispatched tasks in dispatch order.
func (q *InMemoryQueue) Dispatched() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]string, 0, len(q.dispatched))
	for _, d := range q.dispatched {
		out = append(out, d.ID)
	}
	return out
}

// InMemoryResourceProbe returns a configurable hard-coded snapshot.
// Useful for tests and for the cmd/vroom-mesh runner so you can see the
// Overseer escalate without standing up real probes.
type InMemoryResourceProbe struct {
	mu   sync.Mutex
	snap ResourceSnapshot
}

// NewInMemoryResourceProbe returns a probe that initially reports zeros.
func NewInMemoryResourceProbe() *InMemoryResourceProbe {
	return &InMemoryResourceProbe{}
}

// Set overrides the snapshot returned by subsequent Snapshot calls.
func (p *InMemoryResourceProbe) Set(s ResourceSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap = s
}

// Snapshot implements ResourceProbe.
func (p *InMemoryResourceProbe) Snapshot(_ context.Context) (ResourceSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap, nil
}

// InMemoryPermissionChecker is a configurable PermissionChecker for tests
// and the cmd/vroom-mesh demo. Permissions are either allowed or denied
// individually; unknown permissions default to allowed.
type InMemoryPermissionChecker struct {
	mu     sync.Mutex
	denied map[string]string // perm token → reason for denial
}

// NewInMemoryPermissionChecker returns a checker that allows all permissions.
func NewInMemoryPermissionChecker() *InMemoryPermissionChecker {
	return &InMemoryPermissionChecker{denied: map[string]string{}}
}

// Deny marks a permission token as denied with the given reason.
func (c *InMemoryPermissionChecker) Deny(perm, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.denied[perm] = reason
}

// Allow removes a previous Deny — the token becomes allowed again.
func (c *InMemoryPermissionChecker) Allow(perm string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.denied, perm)
}

// Check implements PermissionChecker.
func (c *InMemoryPermissionChecker) Check(_ context.Context, perms []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range perms {
		if reason, denied := c.denied[p]; denied {
			return fmt.Errorf("permission %q denied: %s", p, reason)
		}
	}
	return nil
}
