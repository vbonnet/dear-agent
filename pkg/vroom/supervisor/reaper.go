package supervisor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// WorkerLiveness classifies a dispatched worker session by what `agm session
// list` reports about its tmux pane. The Orchestrator's WorkerReaper uses this
// to decide whether — and how — to reap a session (ce-76bc).
//
// The values mirror the symbols an operator sees in `agm session list`:
//
//	WorkerActive   live tmux pane, making progress (or idle but attached)
//	WorkerStopped  ○ — exited cleanly but the session record was never archived
//	WorkerUnknown  ? — agm cannot determine the state (ghost record)
//	WorkerOrphaned no tmux pane at all, yet the session is still in the registry
type WorkerLiveness string

const (
	// WorkerActive is a session with a live tmux pane.
	WorkerActive WorkerLiveness = "active"

	// WorkerStopped is the ○ state: the process exited but the session
	// record was never archived. A stopped worker older than the reaper's
	// threshold is a zombie.
	WorkerStopped WorkerLiveness = "stopped"

	// WorkerUnknown is the ? state: agm cannot determine the session's
	// liveness. Treated as a ghost and reaped on sight — there is no live
	// pane to drain gracefully.
	WorkerUnknown WorkerLiveness = "unknown"

	// WorkerOrphaned is a session present in the agm registry with no tmux
	// pane backing it. Like WorkerUnknown it has nothing to drain, so it is
	// reaped immediately.
	WorkerOrphaned WorkerLiveness = "orphaned"
)

// WorkerSession is one dispatched worker as observed by the WorkerReaper.
type WorkerSession struct {
	// SessionID is the agm session identifier (e.g. "worker-ce-76bc").
	SessionID string

	// BeadID is the bead the worker was dispatched against. May be empty if
	// the session name does not encode one.
	BeadID string

	// Liveness is what `agm session list` reports about the tmux pane.
	Liveness WorkerLiveness

	// LastActivity is the most-recent observed activity (heartbeat, token
	// output, or state transition). The reaper uses now-LastActivity to age
	// out WorkerStopped sessions. A zero value is treated as "older than any
	// threshold" — a stopped session we cannot date is assumed stale.
	LastActivity time.Time
}

// WorkerLister enumerates the worker sessions the Orchestrator dispatched.
// The production adapter shells out to `agm session list` and filters to
// names beginning with "worker-"; tests use InMemoryWorkerLister.
type WorkerLister interface {
	Workers(ctx context.Context) ([]WorkerSession, error)
}

// WorkerArchiver reaps a single worker session. The production adapter shells
// out to `agm session archive` (plain) or `agm session archive --async`
// (graceful, for sessions with a live pane).
type WorkerArchiver interface {
	// Archive reaps sessionID. When async is true the caller wants a graceful
	// reaper (the session still has a live pane that must drain); when false
	// the session is stopped/ghosted and can be archived immediately.
	Archive(ctx context.Context, sessionID string, async bool) error
}

// BeadStatusChecker reports whether a bead has been closed. The production
// adapter shells out to `bd --db ~/beads/context-engine/.beads show <id>`.
// A nil checker disables the closed-bead reap rule.
type BeadStatusChecker interface {
	IsClosed(ctx context.Context, beadID string) (bool, error)
}

// defaultStoppedReapThreshold is the age after which a WorkerStopped session
// is reaped. It is deliberately short — a cleanly-exiting worker's own
// SessionEnd hook archives it well within this window, so anything still
// stopped-and-unarchived after it is a zombie the hook missed (OOM, kill -9,
// crashed host). The roadmap P0 (closed-bead workers persisting 24h+) is the
// failure this guards against; the threshold reaps long before 24h.
const defaultStoppedReapThreshold = 15 * time.Minute

// ReapResult reports the outcome of a single WorkerReaper.Reap pass.
type ReapResult struct {
	// Scanned is the number of worker sessions inspected.
	Scanned int

	// Reaped is the number successfully archived.
	Reaped int

	// Failed is the number whose archive call returned an error.
	Failed int
}

// WorkerReaper deterministically reaps zombie and ghost worker sessions on
// each Orchestrator tick (ce-76bc). It is the dispatch-side analogue of the
// Overseer's ResourceReclaimer: where the Overseer reclaims OS resources
// (orphaned processes, stranded worktrees), the WorkerReaper reclaims agm
// session slots held by workers that are no longer doing useful work.
//
// A worker is reaped when it is in any of these states:
//
//   - its bead is closed but the session is still around (the P0 directive:
//     closed-bead workers persisting for hours). Active → reaped with --async
//     so the pane drains; otherwise reaped immediately.
//   - it is WorkerUnknown (?) or WorkerOrphaned (no pane) — a ghost record
//     with nothing to drain, reaped immediately.
//   - it is WorkerStopped (○) and has been so for longer than the configured
//     threshold — a zombie the SessionEnd hook failed to clean up.
//
// Every reap is "act after advising": a supervisor.orch.reap record is
// written to the decision trail *before* the archive call, so the trail
// explains why each session was reaped even if the archive itself fails.
type WorkerReaper struct {
	lister    WorkerLister
	archiver  WorkerArchiver
	beads     BeadStatusChecker // nil → closed-bead rule disabled
	trail     decisiontrail.Trail
	threshold time.Duration    // 0 → defaultStoppedReapThreshold
	now       func() time.Time // nil → time.Now
}

// NewWorkerReaper constructs a WorkerReaper. trail, lister, and archiver are
// required. A nil BeadStatusChecker disables the closed-bead reap rule (the
// stopped/ghost rules still apply). A zero threshold uses
// defaultStoppedReapThreshold.
func NewWorkerReaper(trail decisiontrail.Trail, lister WorkerLister, archiver WorkerArchiver) (*WorkerReaper, error) {
	if trail == nil {
		return nil, errors.New("supervisor: WorkerReaper requires a Trail")
	}
	if lister == nil {
		return nil, errors.New("supervisor: WorkerReaper requires a WorkerLister")
	}
	if archiver == nil {
		return nil, errors.New("supervisor: WorkerReaper requires a WorkerArchiver")
	}
	return &WorkerReaper{trail: trail, lister: lister, archiver: archiver}, nil
}

// WithBeadStatusChecker enables the closed-bead reap rule.
func (r *WorkerReaper) WithBeadStatusChecker(b BeadStatusChecker) *WorkerReaper {
	r.beads = b
	return r
}

// WithStoppedThreshold overrides the age after which a WorkerStopped session
// is reaped. A non-positive value resets to the default.
func (r *WorkerReaper) WithStoppedThreshold(d time.Duration) *WorkerReaper {
	r.threshold = d
	return r
}

// withClock overrides the time source (tests inject a fake clock).
func (r *WorkerReaper) withClock(now func() time.Time) {
	r.now = now
}

func (r *WorkerReaper) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *WorkerReaper) stoppedThreshold() time.Duration {
	if r.threshold > 0 {
		return r.threshold
	}
	return defaultStoppedReapThreshold
}

// Reap inspects every worker session and archives the zombies and ghosts.
// It is best-effort: a failed archive is recorded and counted but does not
// abort the pass. The returned error is non-nil only when the worker list
// itself could not be obtained (the Orchestrator treats Reap as best-effort
// and never fails a tick on it).
func (r *WorkerReaper) Reap(ctx context.Context) (ReapResult, error) {
	workers, err := r.lister.Workers(ctx)
	if err != nil {
		_ = r.trail.Append(ctx, decisiontrail.Record{
			Role:    string(RoleOrchestrator),
			Kind:    "supervisor.orch.reap_error",
			Payload: map[string]any{"error": err.Error()},
		})
		return ReapResult{}, err
	}

	now := r.clock()
	thresh := r.stoppedThreshold()
	res := ReapResult{Scanned: len(workers)}

	for _, w := range workers {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}

		reason, async, reap := r.classify(ctx, w, now, thresh)
		if !reap {
			continue
		}

		mode := "plain"
		if async {
			mode = "async"
		}

		// Advise: record the decision before acting so the trail explains
		// the reap even if the archive call fails.
		_ = r.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOrchestrator),
			Kind: "supervisor.orch.reap",
			Payload: map[string]any{
				"session_id": w.SessionID,
				"bead_id":    w.BeadID,
				"liveness":   string(w.Liveness),
				"reason":     reason,
				"mode":       mode,
			},
		})

		if archiveErr := r.archiver.Archive(ctx, w.SessionID, async); archiveErr != nil {
			res.Failed++
			_ = r.trail.Append(ctx, decisiontrail.Record{
				Role: string(RoleOrchestrator),
				Kind: "supervisor.orch.reap_failed",
				Payload: map[string]any{
					"session_id": w.SessionID,
					"bead_id":    w.BeadID,
					"reason":     reason,
					"mode":       mode,
					"error":      archiveErr.Error(),
				},
			})
			continue
		}
		res.Reaped++
	}

	return res, nil
}

// classify applies the deterministic reap rules to one worker and returns
// (reason, async, shouldReap). The rules are evaluated in priority order:
//
//  1. Closed bead — reap regardless of liveness. An active worker drains via
//     --async; a stopped/ghost one is archived immediately.
//  2. Unknown / orphaned — a ghost with nothing to drain; reap immediately.
//  3. Stopped past threshold — a zombie the SessionEnd hook missed.
//
// Anything else (an active worker on an open bead) is left running.
func (r *WorkerReaper) classify(ctx context.Context, w WorkerSession, now time.Time, thresh time.Duration) (reason string, async, reap bool) {
	// Rule 1: closed bead.
	if r.beads != nil && w.BeadID != "" {
		if closed, err := r.beads.IsClosed(ctx, w.BeadID); err == nil && closed {
			// Active sessions drain gracefully; everything else is already
			// dead enough to archive immediately.
			return "bead-closed", w.Liveness == WorkerActive, true
		}
	}

	// Rule 2: ghost records — no live pane to drain.
	switch w.Liveness {
	case WorkerUnknown:
		return "ghost-unknown-state", false, true
	case WorkerOrphaned:
		return "ghost-no-tmux-pane", false, true
	case WorkerStopped:
		// Rule 3: stopped-and-unarchived past the threshold. A zero
		// LastActivity is treated as older than any threshold.
		if w.LastActivity.IsZero() || now.Sub(w.LastActivity) >= thresh {
			return "stopped-stale", false, true
		}
	case WorkerActive:
		// An active worker on an open bead is doing its job — leave it.
	}

	return "", false, false
}

// InMemoryWorkerLister is a test double for WorkerLister.
type InMemoryWorkerLister struct {
	mu      sync.Mutex
	workers []WorkerSession
	err     error
}

// NewInMemoryWorkerLister returns a lister reporting the given workers.
func NewInMemoryWorkerLister(workers ...WorkerSession) *InMemoryWorkerLister {
	return &InMemoryWorkerLister{workers: workers}
}

// Set replaces the reported worker set.
func (l *InMemoryWorkerLister) Set(workers []WorkerSession) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.workers = workers
}

// SetError configures Workers to return err.
func (l *InMemoryWorkerLister) SetError(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.err = err
}

// Workers implements WorkerLister.
func (l *InMemoryWorkerLister) Workers(_ context.Context) ([]WorkerSession, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return nil, l.err
	}
	out := make([]WorkerSession, len(l.workers))
	copy(out, l.workers)
	return out, nil
}

// ArchiveCall records one Archive invocation.
type ArchiveCall struct {
	SessionID string
	Async     bool
}

// InMemoryWorkerArchiver is a test double for WorkerArchiver. It records every
// call and can be configured to fail specific sessions.
type InMemoryWorkerArchiver struct {
	mu        sync.Mutex
	calls     []ArchiveCall
	failFor   map[string]error
	defaultEr error
}

// NewInMemoryWorkerArchiver returns an archiver that succeeds by default.
func NewInMemoryWorkerArchiver() *InMemoryWorkerArchiver {
	return &InMemoryWorkerArchiver{failFor: map[string]error{}}
}

// FailSession makes Archive return err for the given session id.
func (a *InMemoryWorkerArchiver) FailSession(sessionID string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failFor[sessionID] = err
}

// SetDefaultError makes Archive return err for every session.
func (a *InMemoryWorkerArchiver) SetDefaultError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.defaultEr = err
}

// Archive implements WorkerArchiver.
func (a *InMemoryWorkerArchiver) Archive(_ context.Context, sessionID string, async bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, ArchiveCall{SessionID: sessionID, Async: async})
	if err, ok := a.failFor[sessionID]; ok {
		return err
	}
	return a.defaultEr
}

// Calls returns the recorded Archive calls in order.
func (a *InMemoryWorkerArchiver) Calls() []ArchiveCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]ArchiveCall(nil), a.calls...)
}

// InMemoryBeadStatusChecker is a test double for BeadStatusChecker.
type InMemoryBeadStatusChecker struct {
	mu     sync.Mutex
	closed map[string]bool
	err    error
}

// NewInMemoryBeadStatusChecker returns a checker reporting every bead open.
func NewInMemoryBeadStatusChecker() *InMemoryBeadStatusChecker {
	return &InMemoryBeadStatusChecker{closed: map[string]bool{}}
}

// SetClosed marks beadID as closed (true) or open (false).
func (c *InMemoryBeadStatusChecker) SetClosed(beadID string, closed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed[beadID] = closed
}

// SetError configures IsClosed to return err.
func (c *InMemoryBeadStatusChecker) SetError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// IsClosed implements BeadStatusChecker.
func (c *InMemoryBeadStatusChecker) IsClosed(_ context.Context, beadID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return false, c.err
	}
	return c.closed[beadID], nil
}
