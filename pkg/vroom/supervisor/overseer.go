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
}

// DefaultEscalationThreshold is the threshold used when none is configured.
var DefaultEscalationThreshold = EscalationThreshold{Fraction: 0.9}

// overSustainedThreshold is the number of consecutive ticks a metric must
// stay above its escalation threshold before supervisor.over.sustained_escalation
// is emitted. Three consecutive ticks signals a resource problem that has not
// been addressed and warrants operator attention beyond a single-tick alert.
const overSustainedThreshold = 3

// Overseer is the CRO-analogue supervisor. Its Tick takes one
// ResourceSnapshot and emits an escalation event for every metric that
// crosses threshold. Real remediation (reaping, killing runaway sessions,
// reclaiming quota) is out of scope for PR 1 — escalations are
// observation-only.
//
// Sustained escalation (ce-6as.80): when a metric remains above threshold for
// overSustainedThreshold consecutive ticks, Overseer also emits
// supervisor.over.sustained_escalation — a higher-severity signal indicating
// the first-tier alert has not been acted on.
type Overseer struct {
	trail            decisiontrail.Trail
	probe            ResourceProbe
	threshold        EscalationThreshold
	consecutiveAbove map[string]int // metric name → consecutive above-threshold ticks
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
	return &Overseer{
		trail:            trail,
		probe:            probe,
		threshold:        threshold,
		consecutiveAbove: make(map[string]int),
	}, nil
}

// Role implements Supervisor.
func (o *Overseer) Role() Role { return RoleOverseer }

// Tick samples the probe and escalates anything that crosses threshold.
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
			"cpu_used_fraction":    snap.CPUUsedFraction,
			"stranded_worktrees":   snap.StrandedWorktrees,
			"orphaned_sessions":    snap.OrphanedSessions,
		},
	})

	// Evaluate each metric and escalate independently.
	o.maybeEscalateFraction(ctx, "disk", snap.DiskUsedFraction)
	o.maybeEscalateFraction(ctx, "memory", snap.MemoryUsedFraction)
	o.maybeEscalateFraction(ctx, "cpu", snap.CPUUsedFraction)
	o.maybeEscalateCount(ctx, "stranded_worktrees", snap.StrandedWorktrees)
	o.maybeEscalateCount(ctx, "orphaned_sessions", snap.OrphanedSessions)
	return nil
}

// maybeEscalateFraction escalates and tracks sustained-escalation for
// fraction-style metrics (disk, memory, cpu).
func (o *Overseer) maybeEscalateFraction(ctx context.Context, name string, v float64) {
	if v < o.threshold.Fraction {
		o.consecutiveAbove[name] = 0
		return
	}
	o.consecutiveAbove[name]++
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.escalated",
		Payload: map[string]any{
			"metric":    name,
			"value":     v,
			"threshold": o.threshold.Fraction,
		},
	})
	if o.consecutiveAbove[name] >= overSustainedThreshold {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOverseer),
			Kind: "supervisor.over.sustained_escalation",
			Payload: map[string]any{
				"metric":            name,
				"consecutive_ticks": o.consecutiveAbove[name],
				"threshold":         overSustainedThreshold,
				"action":            "investigate: metric has not dropped below threshold after repeated alerts",
			},
		})
	}
}

// maybeEscalateCount escalates and tracks sustained-escalation for count-style
// metrics (stranded_worktrees, orphaned_sessions).
func (o *Overseer) maybeEscalateCount(ctx context.Context, name string, n int) {
	if n <= 0 {
		o.consecutiveAbove[name] = 0
		return
	}
	o.consecutiveAbove[name]++
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.escalated",
		Payload: map[string]any{
			"metric": name,
			"count":  n,
		},
	})
	if o.consecutiveAbove[name] >= overSustainedThreshold {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOverseer),
			Kind: "supervisor.over.sustained_escalation",
			Payload: map[string]any{
				"metric":            name,
				"consecutive_ticks": o.consecutiveAbove[name],
				"threshold":         overSustainedThreshold,
				"action":            "investigate: metric has not been remediated after repeated alerts",
			},
		})
	}
}
