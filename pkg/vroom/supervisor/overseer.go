package supervisor

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// ResourceProbe is the seam the Overseer drives. Per CONTEXT.md §"The three
// supervisors":
//
//	Overseer | Owns: resource usage (CPU/disk/memory/quota), leak detection,
//	                session cleanup.
//
// PR 1 ships an in-memory implementation (InMemoryResourceProbe). Follow-ups
// add real probes that read disk/process/quota state and a SessionReaper
// adapter that invokes `agm worktree sweep` / `agm session gc`.
type ResourceProbe interface {
	// Snapshot returns the current resource state. The supervisor decides
	// (per its policy) whether any metric crosses an escalation threshold.
	Snapshot(ctx context.Context) (ResourceSnapshot, error)
}

// ResourceSnapshot is one observation of the system's resource posture.
// Fields are 0..1 fractions where applicable so policy is uniform across
// metrics. PR 1's policy is "any fraction ≥ 0.9 is an escalation".
type ResourceSnapshot struct {
	// DiskUsedFraction is fraction of disk space used (0..1).
	DiskUsedFraction float64

	// MemoryUsedFraction is fraction of memory used (0..1).
	MemoryUsedFraction float64

	// SwapUsedFraction is fraction of swap space used (0..1). Swap pressure
	// is a leading indicator of memory exhaustion — escalation threshold is
	// lower than the RAM threshold (see EscalationThreshold.SwapFraction).
	SwapUsedFraction float64

	// CPUUsedFraction is fraction of CPU used over the most recent
	// observation window (0..1).
	CPUUsedFraction float64

	// StrandedWorktrees counts worktrees whose branches have been merged
	// but the worktree itself has not been reaped. A leak indicator per
	// memory `dear-agent-worktree-stop-reaper.md`.
	StrandedWorktrees int

	// OrphanedSessions counts AGM sessions whose owning workspace no
	// longer exists. Another leak indicator.
	OrphanedSessions int
}

// EscalationThreshold is the PR-1 default escalation policy. Any single
// fraction-style metric ≥ Fraction triggers an escalation; any count-style
// metric > 0 triggers an escalation.
type EscalationThreshold struct {
	// Fraction is the escalation threshold for Disk/Memory/CPU. Default
	// 0.9 if zero.
	Fraction float64

	// SwapFraction is the escalation threshold specifically for swap pressure.
	// Swap is a leading indicator, so a lower threshold (e.g. 0.5) is
	// appropriate — high swap means the system is already paging out and
	// spawning new heavy processes will degrade performance further.
	// Default 0.5 if zero.
	SwapFraction float64
}

// DefaultEscalationThreshold is the threshold used when none is configured.
var DefaultEscalationThreshold = EscalationThreshold{Fraction: 0.9, SwapFraction: 0.5}

// Overseer is the CRO-analogue supervisor. Its Tick takes one
// ResourceSnapshot and emits an escalation event for every metric that
// crosses threshold. Real remediation (reaping, killing runaway sessions,
// reclaiming quota) is out of scope for PR 1 — escalations are
// observation-only.
//
// When WithBurndown is called, each Tick also runs the burndown concurrency
// maintenance loop: reconcile stale in_progress beads, reclaim them to open,
// and spawn new workers up to the policy target.
type Overseer struct {
	trail          decisiontrail.Trail
	probe          ResourceProbe
	threshold      EscalationThreshold
	burndownCtrl   BurndownController // nil = burndown disabled
	burndownPolicy BurndownPolicy
}

// NewOverseer constructs the Overseer supervisor. If threshold has zero
// fields, DefaultEscalationThreshold is used.
func NewOverseer(trail decisiontrail.Trail, probe ResourceProbe, threshold EscalationThreshold) (*Overseer, error) {
	if trail == nil {
		return nil, errors.New("supervisor: Overseer requires a Trail")
	}
	if probe == nil {
		return nil, errors.New("supervisor: Overseer requires a ResourceProbe")
	}
	if threshold.Fraction <= 0 {
		threshold = DefaultEscalationThreshold
	}
	return &Overseer{trail: trail, probe: probe, threshold: threshold}, nil
}

// WithBurndown wires a BurndownController and policy into the Overseer.
// After this call each Tick also runs the burndown maintenance phase. The
// controller and policy may be changed between ticks; the Overseer copies the
// references at construction and does not hold a lock (callers must not
// replace them during an active Tick).
func (o *Overseer) WithBurndown(ctrl BurndownController, policy BurndownPolicy) *Overseer {
	o.burndownCtrl = ctrl
	o.burndownPolicy = defaultBurndownPolicy(policy)
	return o
}

// Role implements Supervisor.
func (o *Overseer) Role() Role { return RoleOverseer }

// Tick samples the probe, escalates anything that crosses threshold, and —
// when a BurndownController is wired — runs the burndown maintenance phase.
func (o *Overseer) Tick(ctx context.Context) error {
	snap, err := o.probe.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("overseer: snapshot: %w", err)
	}

	// Record the raw snapshot regardless of thresholds — the trail is
	// the place that operators learn the system was OK at this tick.
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.resource_snapshot",
		Payload: map[string]any{
			"disk_used_fraction":   snap.DiskUsedFraction,
			"memory_used_fraction": snap.MemoryUsedFraction,
			"swap_used_fraction":   snap.SwapUsedFraction,
			"cpu_used_fraction":    snap.CPUUsedFraction,
			"stranded_worktrees":   snap.StrandedWorktrees,
			"orphaned_sessions":    snap.OrphanedSessions,
		},
	})

	// Evaluate each metric and escalate independently.
	o.maybeEscalateFraction(ctx, "disk", snap.DiskUsedFraction)
	o.maybeEscalateFraction(ctx, "memory", snap.MemoryUsedFraction)
	o.maybeEscalateSwapFraction(ctx, snap.SwapUsedFraction)
	o.maybeEscalateFraction(ctx, "cpu", snap.CPUUsedFraction)
	o.maybeEscalateCount(ctx, "stranded_worktrees", snap.StrandedWorktrees)
	o.maybeEscalateCount(ctx, "orphaned_sessions", snap.OrphanedSessions)

	// Burndown maintenance phase — only runs when a controller is wired.
	if o.burndownCtrl != nil {
		var spawned int
		o.runBurndownTick(ctx, snap, o.burndownPolicy, o.burndownCtrl, &spawned)
	}
	return nil
}

func (o *Overseer) maybeEscalateFraction(ctx context.Context, name string, v float64) {
	if v < o.threshold.Fraction {
		return
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.escalated",
		Payload: map[string]any{
			"metric":    name,
			"value":     v,
			"threshold": o.threshold.Fraction,
		},
	})
}

// maybeEscalateSwapFraction escalates when swap pressure exceeds
// EscalationThreshold.SwapFraction (default 0.5). Swap uses a lower threshold
// than RAM because high swap is a leading indicator: if the system is already
// paging, spawning new heavy processes will cause thrashing.
func (o *Overseer) maybeEscalateSwapFraction(ctx context.Context, v float64) {
	thresh := o.threshold.SwapFraction
	if thresh <= 0 {
		thresh = DefaultEscalationThreshold.SwapFraction
	}
	if v < thresh {
		return
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.escalated",
		Payload: map[string]any{
			"metric":    "swap",
			"value":     v,
			"threshold": thresh,
		},
	})
}

func (o *Overseer) maybeEscalateCount(ctx context.Context, name string, n int) {
	if n <= 0 {
		return
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.escalated",
		Payload: map[string]any{
			"metric": name,
			"count":  n,
		},
	})
}
