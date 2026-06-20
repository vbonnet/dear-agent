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

	// FreePhysicalMemoryBytes is the absolute amount of free physical RAM in
	// bytes (genuinely free pages, not inactive/evictable). The
	// MemoryPressureMonitor uses it for the emergency free-memory floor
	// (default <200 MiB) because a fraction alone hides how little headroom
	// remains on a large-RAM machine. A value of 0 means the probe could not
	// measure it (treated as "unknown", never as "0 bytes free").
	FreePhysicalMemoryBytes uint64

	// SwapUsedFraction is fraction of swap space used (0..1). Swap pressure
	// is a leading indicator of memory exhaustion — escalation threshold is
	// lower than the RAM threshold (see EscalationThreshold.SwapFraction).
	SwapUsedFraction float64

	// CPUUsedFraction is fraction of CPU used over the most recent
	// observation window (0..1).
	CPUUsedFraction float64

	// OpenFDFraction is the fraction of the system-wide open-file descriptor
	// limit currently in use (0..1). Values approaching 1.0 indicate
	// imminent FD exhaustion. On Darwin this reads kern.num_files /
	// kern.maxfiles; on Linux it reads /proc/sys/fs/file-nr.
	OpenFDFraction float64

	// VnodeUsedFraction is the fraction of the kernel vnode table currently
	// in use (Darwin only; 0 on other platforms). Vnode exhaustion is the
	// root cause of the "package errors is not in std" build failures —
	// at 1.0 the kernel can no longer allocate vnodes and filesystem ops
	// start failing (see memory/gopls-fd-leak-blocks-go-build.md).
	VnodeUsedFraction float64

	// GoplsProcesses counts *orphaned* gopls processes — those reparented to
	// PID 1 after their owning Claude session died (not live gopls bound to a
	// running session, which would make the count scale with session count and
	// fire phantom alarms — see ce-u7v9). Each orphaned gopls instance holds
	// ~4800 FDs; accumulation is the primary driver of FD/vnode exhaustion.
	// Escalation fires when the count exceeds EscalationThreshold.GoplsProcesses.
	GoplsProcesses int

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

	// GoplsProcesses is the count above which the Overseer escalates on
	// orphaned-gopls accumulation. Default 5 if zero — more than 5 gopls
	// reparented to PID 1 is a clear leak (each holds ~4800 FDs on Darwin).
	// Note this counts orphans only, not live session gopls (see
	// ResourceSnapshot.GoplsProcesses).
	GoplsProcesses int
}

// DefaultEscalationThreshold is the threshold used when none is configured.
var DefaultEscalationThreshold = EscalationThreshold{Fraction: 0.9, SwapFraction: 0.5, GoplsProcesses: 5}

// ResourceReclaimer is the remediation seam the Overseer calls when resource
// pressure exceeds threshold. Implementations kill orphaned processes,
// reap stranded worktrees, or archive dead sessions.
//
// Reclaim is best-effort: a partial success (some orphans killed, others
// failed) is reported in ReclaimResult and does not cause Tick to fail.
type ResourceReclaimer interface {
	Reclaim(ctx context.Context) (ReclaimResult, error)
}

// ReclaimResult reports the outcome of a single remediation pass.
type ReclaimResult struct {
	OrphansFound  int
	OrphansKilled int
	OrphansFailed int
}

// Overseer is the CRO-analogue supervisor. Its Tick takes one
// ResourceSnapshot, emits escalation events for metrics that cross threshold,
// and — when a ResourceReclaimer is wired — remediates by killing orphaned
// processes and re-snapshotting to verify the pressure dropped.
//
// When WithBurndown is called, each Tick also runs the burndown concurrency
// maintenance loop: reconcile stale in_progress beads, reclaim them to open,
// and spawn new workers up to the policy target.
type Overseer struct {
	trail          decisiontrail.Trail
	probe          ResourceProbe
	threshold      EscalationThreshold
	reclaimer      ResourceReclaimer  // nil = remediation disabled
	burndownCtrl   BurndownController // nil = burndown disabled
	burndownPolicy BurndownPolicy
	prCounter      OpenPRCounter // nil = open-PR cap disabled

	// Graduated memory-pressure monitoring (ce-80ca). Both are set together by
	// WithMemoryPressure; nil = graduated pressure handling disabled (the
	// binary EscalationThreshold path still runs).
	pressureMonitor *MemoryPressureMonitor
	pressureReaper  *AutoResourceReaper
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
		threshold.Fraction = DefaultEscalationThreshold.Fraction
	}
	if threshold.SwapFraction <= 0 {
		threshold.SwapFraction = DefaultEscalationThreshold.SwapFraction
	}
	if threshold.GoplsProcesses <= 0 {
		threshold.GoplsProcesses = DefaultEscalationThreshold.GoplsProcesses
	}
	return &Overseer{trail: trail, probe: probe, threshold: threshold}, nil
}

// WithReclaimer wires a ResourceReclaimer into the Overseer. After this
// call, Tick calls Reclaim when any resource-pressure metric (FDs, vnodes,
// gopls count) exceeds its escalation threshold, then re-snapshots to verify
// the pressure dropped.
func (o *Overseer) WithReclaimer(r ResourceReclaimer) *Overseer {
	o.reclaimer = r
	return o
}

// WithMemoryPressure wires graduated memory-pressure monitoring (ce-80ca)
// into the Overseer. After this call each Tick classifies the snapshot through
// the monitor and, on a warn/critical/emergency breach, drives the reaper's
// graduated remediation and (for critical+) emits a DEAR retro to the trail.
// This runs alongside — not instead of — the binary EscalationThreshold path.
func (o *Overseer) WithMemoryPressure(monitor *MemoryPressureMonitor, reaper *AutoResourceReaper) *Overseer {
	o.pressureMonitor = monitor
	o.pressureReaper = reaper
	return o
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

// WithOpenPRCounter wires an OpenPRCounter into the Overseer, arming the
// open-PR backpressure valve (ce-qpg9). After this call each burndown tick
// queries the counter before spawning: when the open-PR count exceeds
// BurndownPolicy.OpenPRCap, dispatch is paused for that tick. With no counter
// wired the cap is not enforced and burndown behaves as before.
func (o *Overseer) WithOpenPRCounter(c OpenPRCounter) *Overseer {
	o.prCounter = c
	return o
}

// Role implements Supervisor.
func (o *Overseer) Role() Role { return RoleOverseer }

// Tick samples the probe, escalates anything that crosses threshold,
// remediates if a reclaimer is wired and resource pressure is high, and —
// when a BurndownController is wired — runs the burndown maintenance phase.
func (o *Overseer) Tick(ctx context.Context) error {
	snap, err := o.probe.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("overseer: snapshot: %w", err)
	}

	o.recordSnapshot(ctx, snap)

	// Evaluate each metric and escalate independently.
	o.maybeEscalateFraction(ctx, "disk", snap.DiskUsedFraction)
	o.maybeEscalateFraction(ctx, "memory", snap.MemoryUsedFraction)
	o.maybeEscalateSwapFraction(ctx, snap.SwapUsedFraction)
	o.maybeEscalateFraction(ctx, "cpu", snap.CPUUsedFraction)
	o.maybeEscalateFraction(ctx, "open_fds", snap.OpenFDFraction)
	o.maybeEscalateFraction(ctx, "vnodes", snap.VnodeUsedFraction)
	o.maybeEscalateGopls(ctx, snap.GoplsProcesses)
	o.maybeEscalateCount(ctx, "stranded_worktrees", snap.StrandedWorktrees)
	o.maybeEscalateCount(ctx, "orphaned_sessions", snap.OrphanedSessions)

	// Resource remediation: when a reclaimer is wired and pressure metrics
	// are above threshold, kill orphaned processes and re-snapshot to verify.
	if o.reclaimer != nil && o.needsReclaim(snap) {
		o.reclaim(ctx, snap)
	}

	// Graduated memory-pressure handling (ce-80ca) — only runs when both the
	// monitor and reaper are wired via WithMemoryPressure.
	if o.pressureMonitor != nil && o.pressureReaper != nil {
		o.runMemoryPressureTick(ctx, snap)
	}

	// Burndown maintenance phase — only runs when a controller is wired.
	if o.burndownCtrl != nil {
		var spawned int
		o.runBurndownTick(ctx, snap, o.burndownPolicy, o.burndownCtrl, &spawned)
	}
	return nil
}

// recordSnapshot writes the raw snapshot to the decision trail regardless of
// thresholds — the trail is the place that operators learn the system was OK.
func (o *Overseer) recordSnapshot(ctx context.Context, snap ResourceSnapshot) {
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.resource_snapshot",
		Payload: map[string]any{
			"disk_used_fraction":   snap.DiskUsedFraction,
			"memory_used_fraction": snap.MemoryUsedFraction,
			"free_physical_bytes":  snap.FreePhysicalMemoryBytes,
			"swap_used_fraction":   snap.SwapUsedFraction,
			"cpu_used_fraction":    snap.CPUUsedFraction,
			"open_fd_fraction":     snap.OpenFDFraction,
			"vnode_used_fraction":  snap.VnodeUsedFraction,
			"gopls_processes":      snap.GoplsProcesses,
			"stranded_worktrees":   snap.StrandedWorktrees,
			"orphaned_sessions":    snap.OrphanedSessions,
		},
	})
}

// needsReclaim reports whether any resource-pressure metric that the reclaimer
// can act on (FDs, vnodes, gopls process count) is above its threshold.
func (o *Overseer) needsReclaim(snap ResourceSnapshot) bool {
	if snap.OpenFDFraction >= o.threshold.Fraction {
		return true
	}
	if snap.VnodeUsedFraction >= o.threshold.Fraction {
		return true
	}
	goplsThresh := o.threshold.GoplsProcesses
	if goplsThresh <= 0 {
		goplsThresh = DefaultEscalationThreshold.GoplsProcesses
	}
	if snap.GoplsProcesses > goplsThresh {
		return true
	}
	return false
}

// reclaim runs the ResourceReclaimer, records the result, and re-snapshots
// to verify the pressure dropped.
func (o *Overseer) reclaim(ctx context.Context, before ResourceSnapshot) {
	result, err := o.reclaimer.Reclaim(ctx)

	payload := map[string]any{
		"orphans_found":  result.OrphansFound,
		"orphans_killed": result.OrphansKilled,
		"orphans_failed": result.OrphansFailed,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role:    string(RoleOverseer),
		Kind:    "supervisor.over.reclaim",
		Payload: payload,
	})

	if result.OrphansKilled == 0 {
		return
	}

	// Re-snapshot to verify pressure dropped.
	after, snapErr := o.probe.Snapshot(ctx)
	if snapErr != nil {
		return
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.reclaim_verify",
		Payload: map[string]any{
			"fd_before":     before.OpenFDFraction,
			"fd_after":      after.OpenFDFraction,
			"vnode_before":  before.VnodeUsedFraction,
			"vnode_after":   after.VnodeUsedFraction,
			"gopls_before":  before.GoplsProcesses,
			"gopls_after":   after.GoplsProcesses,
			"pressure_down": after.OpenFDFraction < before.OpenFDFraction || after.GoplsProcesses < before.GoplsProcesses,
		},
	})
}

// runMemoryPressureTick classifies the snapshot through the pressure monitor
// and, on a breach, drives the reaper's graduated remediation. For critical+
// incidents it re-snapshots after remediation and emits a DEAR retro so the
// Audit/Retro phases capture whether the pressure actually eased. Warn-level
// breaches are log-only (the reaper records the observation; no DEAR retro is
// warranted for a non-actioned warning).
func (o *Overseer) runMemoryPressureTick(ctx context.Context, snap ResourceSnapshot) {
	level := o.pressureMonitor.Classify(snap)
	if level == PressureNone {
		return
	}

	outcome := o.pressureReaper.React(ctx, level, snap)

	// Warn is log-only — React already recorded the observation.
	if level < PressureCritical {
		return
	}

	// Critical+: re-snapshot to audit whether remediation eased pressure, then
	// emit the DEAR retro. A failed re-snapshot falls back to the before-state
	// so the retro still records the incident (Audit will read "did not ease").
	after, err := o.probe.Snapshot(ctx)
	if err != nil {
		after = snap
	}
	_ = o.trail.Append(ctx, ResourceIncidentRetro(ResourceIncident{
		Level:   level,
		Before:  snap,
		After:   after,
		Outcome: outcome,
	}))
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

// maybeEscalateGopls escalates when the gopls process count exceeds
// EscalationThreshold.GoplsProcesses (default 5). Unlike the generic count
// escalation which fires at >0, gopls processes have a higher normal baseline.
func (o *Overseer) maybeEscalateGopls(ctx context.Context, n int) {
	thresh := o.threshold.GoplsProcesses
	if thresh <= 0 {
		thresh = DefaultEscalationThreshold.GoplsProcesses
	}
	if n <= thresh {
		return
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.escalated",
		Payload: map[string]any{
			"metric":    "gopls_processes",
			"count":     n,
			"threshold": thresh,
		},
	})
}
