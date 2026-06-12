package supervisor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// BurndownController is the seam the Overseer drives to keep N burndown
// workers alive. Mirrors ResourceProbe: the Overseer owns policy; the
// controller owns the side effects (listing/spawning/reclaiming).
//
// PR-1 (this change) ships InMemoryBurndownController only — pure observation
// with no real side effects. PR-2 will wire in AGM/beads adapters.
type BurndownController interface {
	// Live reports the burndown worker sessions the controller believes are
	// actively making progress (heartbeat/commit within the freshness window).
	Live(ctx context.Context) ([]BurndownSession, error)

	// Reconcile maps in_progress beads to Live sessions and returns beads
	// whose owning session is dead (no live session, no commit within
	// staleAfter). These are candidates to reclaim to open and retry.
	Reconcile(ctx context.Context, staleAfter time.Duration) ([]StaleBead, error)

	// Reclaim moves a stale bead back to open so it can be re-dispatched.
	Reclaim(ctx context.Context, beadID string) error

	// Spawn starts one burndown worker against the given spec, returning the
	// new session id. The workspace MUST be a ~/worktrees path (never ~/src)
	// — the adapter enforces this by construction.
	Spawn(ctx context.Context, spec WorkerSpec) (sessionID string, err error)
}

// BurndownSession describes one live burndown worker session.
type BurndownSession struct {
	// SessionID is the AGM session identifier.
	SessionID string

	// Model is the model the session is running (e.g. claude-opus-4-8).
	Model string

	// BeadID is the bead the session has claimed, if any.
	BeadID string

	// LastHeartbeat is the most-recent heartbeat from the session.
	LastHeartbeat time.Time
}

// StaleBead is a bead that was in_progress but whose owning session is dead.
type StaleBead struct {
	// BeadID is the bead tracker identifier.
	BeadID string

	// SessionID is the dead session that had claimed this bead.
	SessionID string

	// Reason is a short diagnostic: "no-live-session" or "no-commit-within-window".
	Reason string
}

// WorkerSpec describes the resources for a new burndown worker.
type WorkerSpec struct {
	// Workspace is the worktree path, e.g. ~/worktrees/dear-agent-burndown-4.
	// Must begin with "~/worktrees/" — Spawn adapters enforce this.
	Workspace string

	// Model is the model to run the worker under.
	Model string

	// Priority is the bead priority class to process: "P0", "P1", "P2".
	Priority string
}

// BurndownPolicy controls how the Overseer maintains burndown concurrency.
type BurndownPolicy struct {
	// Target is the desired number of concurrent burndown workers. Default 3.
	Target int

	// StaleAfter is the time after which a claimed bead with no progress is
	// considered orphaned. Default 20 minutes.
	StaleAfter time.Duration

	// Models is the ordered list of models to assign to workers (round-robin).
	// Default ["claude-opus-4-8"].
	Models []string
}

// defaultBurndownPolicy fills zero fields with defaults.
func defaultBurndownPolicy(p BurndownPolicy) BurndownPolicy {
	if p.Target <= 0 {
		p.Target = 3
	}
	if p.StaleAfter <= 0 {
		p.StaleAfter = 20 * time.Minute
	}
	if len(p.Models) == 0 {
		p.Models = []string{"claude-opus-4-8"}
	}
	return p
}

// runBurndownTick runs the burndown phase of the Overseer's Tick. It:
//  1. Lists live workers.
//  2. Reconciles stale in_progress beads and reclaims them.
//  3. Spawns new workers up to policy.Target (if resources allow).
//  4. Emits one decisiontrail record per action.
//
// snap is the resource snapshot already taken by the main Tick; spawn is
// suppressed when any fraction metric is at or above threshold.
func (o *Overseer) runBurndownTick(ctx context.Context, snap ResourceSnapshot, policy BurndownPolicy, ctrl BurndownController, spawned *int) {
	// 1. Observe live sessions.
	live, err := ctrl.Live(ctx)
	if err != nil {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role:    string(RoleOverseer),
			Kind:    "burndown.live_error",
			Payload: map[string]any{"error": err.Error()},
		})
		return
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role:    string(RoleOverseer),
		Kind:    "burndown.live",
		Payload: map[string]any{"count": len(live)},
	})

	// 2. Reconcile stale beads.
	stale, err := ctrl.Reconcile(ctx, policy.StaleAfter)
	if err != nil {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role:    string(RoleOverseer),
			Kind:    "burndown.reconcile_error",
			Payload: map[string]any{"error": err.Error()},
		})
		return
	}
	for _, sb := range stale {
		if rErr := ctrl.Reclaim(ctx, sb.BeadID); rErr != nil {
			_ = o.trail.Append(ctx, decisiontrail.Record{
				Role: string(RoleOverseer),
				Kind: "burndown.reclaim_error",
				Payload: map[string]any{
					"bead_id":    sb.BeadID,
					"session_id": sb.SessionID,
					"error":      rErr.Error(),
				},
			})
			continue
		}
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOverseer),
			Kind: "burndown.reclaim",
			Payload: map[string]any{
				"bead_id":    sb.BeadID,
				"session_id": sb.SessionID,
				"reason":     sb.Reason,
			},
		})
	}

	// 3. Maintain concurrency. Live count after reclaim = live − any that were
	// stale (approximation: we don't re-query after reclaim in PR-1).
	liveAfter := len(live) - len(stale)
	if liveAfter < 0 {
		liveAfter = 0
	}
	deficit := policy.Target - liveAfter
	if deficit <= 0 {
		return // at or above target — nothing to do
	}

	// Suppress spawn when resources are saturated.
	if resourcesSaturated(snap, o.threshold) {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role:    string(RoleOverseer),
			Kind:    "burndown.spawn_blocked",
			Payload: map[string]any{"reason": "resource_threshold"},
		})
		return
	}

	*spawned = 0
	for i := 0; i < deficit; i++ {
		model := policy.Models[(*spawned)%len(policy.Models)]
		spec := WorkerSpec{
			Workspace: fmt.Sprintf("~/worktrees/dear-agent-burndown-%d", len(live)+*spawned+1),
			Model:     model,
			Priority:  "P1",
		}
		sid, sErr := ctrl.Spawn(ctx, spec)
		if sErr != nil {
			_ = o.trail.Append(ctx, decisiontrail.Record{
				Role: string(RoleOverseer),
				Kind: "burndown.spawn_error",
				Payload: map[string]any{
					"workspace": spec.Workspace,
					"error":     sErr.Error(),
				},
			})
			break
		}
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOverseer),
			Kind: "burndown.spawn",
			Payload: map[string]any{
				"session_id": sid,
				"workspace":  spec.Workspace,
				"model":      model,
			},
		})
		*spawned++
	}
}

// resourcesSaturated returns true if any fraction metric meets or exceeds the
// threshold, meaning the Overseer should not spawn additional work.
func resourcesSaturated(snap ResourceSnapshot, t EscalationThreshold) bool {
	thresh := t.Fraction
	if thresh <= 0 {
		thresh = DefaultEscalationThreshold.Fraction
	}
	return snap.DiskUsedFraction >= thresh ||
		snap.MemoryUsedFraction >= thresh ||
		snap.CPUUsedFraction >= thresh
}

// InMemoryBurndownController is the PR-1 test double for BurndownController.
// It holds sessions and beads in memory with no external I/O. Real adapters
// (AGM/beads) ship in PR-2.
type InMemoryBurndownController struct {
	mu       sync.Mutex
	sessions []BurndownSession
	beads    []inMemBead
}

type inMemBead struct {
	id        string
	sessionID string
	claimedAt time.Time
}

// AddSession registers a live session in the controller.
func (c *InMemoryBurndownController) AddSession(s BurndownSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions = append(c.sessions, s)
}

// ClaimBead records that the given session has claimed the given bead.
func (c *InMemoryBurndownController) ClaimBead(beadID, sessionID string, claimedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.beads = append(c.beads, inMemBead{id: beadID, sessionID: sessionID, claimedAt: claimedAt})
}

// Live implements BurndownController.
func (c *InMemoryBurndownController) Live(_ context.Context) ([]BurndownSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]BurndownSession, len(c.sessions))
	copy(out, c.sessions)
	return out, nil
}

// Reconcile implements BurndownController. A bead is stale when no live
// session has a matching SessionID and the bead was claimed before staleAfter
// ago.
func (c *InMemoryBurndownController) Reconcile(_ context.Context, staleAfter time.Duration) ([]StaleBead, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	liveIDs := make(map[string]struct{}, len(c.sessions))
	for _, s := range c.sessions {
		liveIDs[s.SessionID] = struct{}{}
	}
	now := time.Now()
	var out []StaleBead
	for _, b := range c.beads {
		if _, ok := liveIDs[b.sessionID]; ok {
			continue // session still live
		}
		if now.Sub(b.claimedAt) < staleAfter {
			continue // too recent to call stale
		}
		out = append(out, StaleBead{
			BeadID:    b.id,
			SessionID: b.sessionID,
			Reason:    "no-live-session",
		})
	}
	return out, nil
}

// Reclaim implements BurndownController. It removes the bead from the
// in-memory store (simulating moving it from in_progress→open).
func (c *InMemoryBurndownController) Reclaim(_ context.Context, beadID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, b := range c.beads {
		if b.id != beadID {
			c.beads[n] = b
			n++
		}
	}
	c.beads = c.beads[:n]
	return nil
}

// Spawn implements BurndownController. Validates the workspace prefix and
// records the session in memory.
func (c *InMemoryBurndownController) Spawn(_ context.Context, spec WorkerSpec) (string, error) {
	if !strings.HasPrefix(spec.Workspace, "~/worktrees/") {
		return "", fmt.Errorf("burndown: Spawn: workspace %q must be under ~/worktrees/", spec.Workspace)
	}
	sid := fmt.Sprintf("inmem-%s-%d", spec.Workspace, time.Now().UnixNano())
	c.mu.Lock()
	c.sessions = append(c.sessions, BurndownSession{
		SessionID:     sid,
		Model:         spec.Model,
		LastHeartbeat: time.Now(),
	})
	c.mu.Unlock()
	return sid, nil
}
