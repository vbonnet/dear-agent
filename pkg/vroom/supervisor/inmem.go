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

// InMemoryOpenPRCounter is a configurable OpenPRCounter for tests and for the
// cmd/vroom-mesh runner, so the open-PR backpressure valve (ce-qpg9) can be
// exercised without a live GitHub repo.
type InMemoryOpenPRCounter struct {
	mu    sync.Mutex
	count int
	err   error
}

// NewInMemoryOpenPRCounter returns a counter that reports the given count.
func NewInMemoryOpenPRCounter(count int) *InMemoryOpenPRCounter {
	return &InMemoryOpenPRCounter{count: count}
}

// Set overrides the count returned by subsequent OpenPRs calls.
func (c *InMemoryOpenPRCounter) Set(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count = count
}

// SetErr makes subsequent OpenPRs calls return the given error.
func (c *InMemoryOpenPRCounter) SetErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// OpenPRs implements OpenPRCounter.
func (c *InMemoryOpenPRCounter) OpenPRs(_ context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count, c.err
}

// InMemoryReclaimer is a configurable ResourceReclaimer for tests.
type InMemoryReclaimer struct {
	mu       sync.Mutex
	result   ReclaimResult
	err      error
	calls    int
	callback func() // called on each Reclaim invocation (e.g. to update probe)
}

// NewInMemoryReclaimer returns a reclaimer that reports the configured result.
func NewInMemoryReclaimer() *InMemoryReclaimer {
	return &InMemoryReclaimer{}
}

// SetResult configures the result returned by Reclaim.
func (r *InMemoryReclaimer) SetResult(result ReclaimResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.result = result
	r.err = err
}

// OnReclaim registers a callback invoked on each Reclaim call (before
// returning the result). Tests use this to simulate the probe reading lower
// values after the reclaim.
func (r *InMemoryReclaimer) OnReclaim(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callback = fn
}

// Reclaim implements ResourceReclaimer.
func (r *InMemoryReclaimer) Reclaim(_ context.Context) (ReclaimResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.callback != nil {
		r.callback()
	}
	return r.result, r.err
}

// Calls returns the number of times Reclaim was called.
func (r *InMemoryReclaimer) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// InMemoryPeerRecovery is a configurable PeerRecovery for tests.
type InMemoryPeerRecovery struct {
	mu    sync.Mutex
	calls []PeerRecoveryCall
	err   error
}

// PeerRecoveryCall records one Recover invocation.
type PeerRecoveryCall struct {
	PeerRole Role
	Reason   string
}

// NewInMemoryPeerRecovery returns a recovery that succeeds by default.
func NewInMemoryPeerRecovery() *InMemoryPeerRecovery {
	return &InMemoryPeerRecovery{}
}

// SetError configures the error returned by subsequent Recover calls.
func (r *InMemoryPeerRecovery) SetError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// Recover implements PeerRecovery.
func (r *InMemoryPeerRecovery) Recover(_ context.Context, peerRole Role, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, PeerRecoveryCall{PeerRole: peerRole, Reason: reason})
	return r.err
}

// Calls returns all recorded recovery calls.
func (r *InMemoryPeerRecovery) Calls() []PeerRecoveryCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]PeerRecoveryCall(nil), r.calls...)
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
