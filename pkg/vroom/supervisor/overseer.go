package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	// MemorystatusLevel is the macOS kernel memory-pressure level from
	// kern.memorystatus_level (0–100; 100 = plenty of memory, <40 = high
	// pressure). Zero on non-Darwin platforms or when the sysctl is
	// unavailable — treated as "unknown" by the monitor.
	MemorystatusLevel int
}

// EscalationThreshold is the PR-1 default escalation policy. Any single
// fraction-style metric ≥ Fraction triggers an escalation; any count-style
// metric > 0 triggers an escalation.
type EscalationThreshold struct {
	// Fraction is the escalation threshold for Disk/Memory/CPU. Default
	// 0.9 if zero.
	Fraction float64

	// SwapFraction is the escalation threshold specifically for swap pressure.
	// Swap is a leading indicator, so a lower threshold than RAM/disk/CPU is
	// appropriate — high swap means the system is already paging out and
	// spawning new heavy processes will degrade performance further.
	// Default 0.75 if zero. (On macOS, vm_stat inactive pages are reclaimable
	// so swap is the reliable pressure signal; 0.75 avoids false alarms from
	// LRU-warm steady states that previously fired at 0.50.)
	SwapFraction float64

	// GoplsProcesses is the count above which the Overseer escalates on
	// orphaned-gopls accumulation. Default 5 if zero — more than 5 gopls
	// reparented to PID 1 is a clear leak (each holds ~4800 FDs on Darwin).
	// Note this counts orphans only, not live session gopls (see
	// ResourceSnapshot.GoplsProcesses).
	GoplsProcesses int
}

// DefaultEscalationThreshold is the threshold used when none is configured.
var DefaultEscalationThreshold = EscalationThreshold{Fraction: 0.9, SwapFraction: 0.75, GoplsProcesses: 5}

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

	// Session hygiene (ce-cxjb.3). When a SessionGardener is wired via
	// WithSessionHygiene, each Tick archives completed/stopped worker sessions
	// via `agm session gc`. nil = hygiene disabled.
	gardener        SessionGardener
	hygieneInterval time.Duration // minimum spacing between passes (rate-limit)
	lastHygiene     time.Time     // clock time of the last pass (zero = never)
	clock           func() time.Time

	// Session inbox-count alert (ce-cxjb.4). When a SessionInboxCounter is
	// wired via WithSessionInboxAlert, each Tick counts non-archived sessions
	// and escalates when the count exceeds inboxAlertThreshold. nil = disabled.
	inboxCounter        SessionInboxCounter
	inboxAlertThreshold int

	// Memory-pressure alert (ce-6jl5). When a MemoryAlertNotifier is wired via
	// WithMemoryAlert, each Tick classifies the snapshot's free-RAM and swap
	// against alertThresholds and routes WARN/CRITICAL alerts to the
	// Meta-Orchestrator (and, at CRITICAL, the Orchestrator). nil = disabled.
	alertNotifier   MemoryAlertNotifier
	alertThresholds MemoryAlertThresholds
}

// defaultHygieneInterval rate-limits the session-hygiene pass so ticks firing
// faster than this don't thrash the gc op. The Overseer runs hygiene at most
// once per tick anyway; this guards against sub-interval ticks.
const defaultHygieneInterval = 30 * time.Second

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

// WithSessionHygiene wires a SessionGardener into the Overseer, arming the
// per-tick session-hygiene pass (ce-cxjb.3). After this call each Tick archives
// completed/stopped worker sessions (via `agm session gc`), rate-limited to one
// pass per interval. A zero interval applies defaultHygieneInterval. The GC is
// idempotent and conservative (active sessions and protected roles are always
// skipped), so it is safe to run on a cadence. With no gardener wired the
// Overseer only probes OrphanedSessions; it does not archive.
func (o *Overseer) WithSessionHygiene(g SessionGardener, interval time.Duration) *Overseer {
	o.gardener = g
	if interval <= 0 {
		interval = defaultHygieneInterval
	}
	o.hygieneInterval = interval
	return o
}

// WithSessionInboxAlert wires a SessionInboxCounter into the Overseer, arming
// the per-tick session inbox-count alert (ce-cxjb.4). After this call each Tick
// counts non-archived sessions (active + stopped, excluding archived) and emits
// an escalation to the trail when the count exceeds threshold. A threshold ≤ 0
// applies defaultSessionInboxAlertThreshold. The count is read-only — it never
// archives or mutates sessions; remediation is the SessionGardener's job
// (WithSessionHygiene). With no counter wired the Overseer does not alert on
// inbox depth.
func (o *Overseer) WithSessionInboxAlert(c SessionInboxCounter, threshold int) *Overseer {
	o.inboxCounter = c
	if threshold <= 0 {
		threshold = defaultSessionInboxAlertThreshold
	}
	o.inboxAlertThreshold = threshold
	return o
}

// WithMemoryAlert wires a MemoryAlertNotifier into the Overseer, arming the
// per-tick memory-pressure alert (ce-6jl5). After this call each Tick classifies
// the snapshot's free physical RAM and swap usage against thresholds and, on a
// WARN/CRITICAL breach, records an alert to the trail and routes it to the
// Meta-Orchestrator — and, at CRITICAL, the Orchestrator too. Any zero field in
// thresholds falls back to DefaultMemoryAlertThresholds (free < 500/200 MiB,
// swap > 50/75%). The alert is observe-and-notify only; remediation remains the
// AutoResourceReaper's job (WithMemoryPressure). With no notifier wired the
// Overseer does not classify or alert on memory pressure.
func (o *Overseer) WithMemoryAlert(n MemoryAlertNotifier, thresholds MemoryAlertThresholds) *Overseer {
	o.alertNotifier = n
	o.alertThresholds = thresholds.withDefaults()
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

	// Session hygiene (ce-cxjb.3) — archive completed/stopped worker sessions.
	// Runs at most once per tick and is rate-limited; only when a gardener is
	// wired via WithSessionHygiene.
	if o.gardener != nil {
		o.runSessionHygiene(ctx)
	}

	// Session inbox-count alert (ce-cxjb.4) — escalate when non-archived
	// sessions accumulate past threshold. Read-only; only when a counter is
	// wired via WithSessionInboxAlert.
	if o.inboxCounter != nil {
		o.runSessionInboxAlert(ctx)
	}

	// Burndown maintenance phase — only runs when a controller is wired.
	if o.burndownCtrl != nil {
		var spawned int
		o.runBurndownTick(ctx, snap, o.burndownPolicy, o.burndownCtrl, &spawned)
	}

	// Memory-pressure alert (ce-6jl5) — classify free-RAM/swap and route
	// WARN/CRITICAL alerts to the supervisors. Runs after the burndown /
	// bead-pr-sync reconcile per the 2026-06-18 retro design; only when a
	// notifier is wired via WithMemoryAlert.
	if o.alertNotifier != nil {
		o.runMemoryAlertTick(ctx, snap)
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
			"memorystatus_level":   snap.MemorystatusLevel,
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

// now returns the current time through the clock seam (time.Now in
// production; a fake in tests).
func (o *Overseer) now() time.Time {
	if o.clock != nil {
		return o.clock()
	}
	return time.Now()
}

// runSessionHygiene archives completed/stopped worker sessions via the wired
// SessionGardener (`agm session gc`), then records the outcome to the trail.
// It is rate-limited: a pass is skipped if the previous one ran less than
// hygieneInterval ago, so ticks firing faster than the interval don't thrash
// the gc op. The gardener's GC is idempotent and conservative — active
// sessions and protected roles are always skipped — so a skipped pass loses
// nothing; the next eligible tick picks up the work.
func (o *Overseer) runSessionHygiene(ctx context.Context) {
	now := o.now()
	if !o.lastHygiene.IsZero() && now.Sub(o.lastHygiene) < o.hygieneInterval {
		return // rate-limited; too soon since the last pass
	}
	o.lastHygiene = now

	stats, err := o.gardener.GC(ctx)

	payload := map[string]any{
		"scanned":  stats.Scanned,
		"archived": stats.Archived,
		"skipped":  stats.Skipped,
		"errors":   stats.Errors,
		"note":     fmt.Sprintf("archived %d sessions", stats.Archived),
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role:    string(RoleOverseer),
		Kind:    "overseer.session.hygiene",
		Payload: payload,
	})
}

// runSessionInboxAlert counts non-archived sessions via the wired
// SessionInboxCounter and escalates to the trail when the count exceeds
// inboxAlertThreshold. Unlike hygiene this is read-only — it observes inbox
// depth so an operator learns sessions are accumulating, but leaves remediation
// to the SessionGardener. A count at or below threshold records nothing (the
// trail stays quiet when the inbox is healthy); a counter error is recorded so
// the failure is visible, but never fails the tick.
func (o *Overseer) runSessionInboxAlert(ctx context.Context) {
	count, err := o.inboxCounter.InboxCount(ctx)
	if err != nil {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOverseer),
			Kind: "overseer.session.inbox_alert",
			Payload: map[string]any{
				"error": err.Error(),
			},
		})
		return
	}

	threshold := o.inboxAlertThreshold
	if threshold <= 0 {
		threshold = defaultSessionInboxAlertThreshold
	}
	if count <= threshold {
		return // inbox is healthy; stay quiet
	}

	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "overseer.session.inbox_alert",
		Payload: map[string]any{
			"count":     count,
			"threshold": threshold,
			"alert": fmt.Sprintf(
				"session inbox contains %d non-archived sessions (threshold: %d); run 'agm session gc' to clean up",
				count, threshold,
			),
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

// runMemoryAlertTick classifies the snapshot's free-RAM and swap against the
// alert thresholds and, on a WARN/CRITICAL breach, records the alert to the
// trail and routes it to the supervisors that can act: every alert goes to the
// Meta-Orchestrator; a CRITICAL alert also goes to the Orchestrator (which owns
// the spawn queue). A healthy snapshot records nothing — the trail stays quiet
// when memory is fine (the raw snapshot is already logged by recordSnapshot).
// Notifier failures are recorded but never fail the tick; under memory pressure
// reaching *one* supervisor beats aborting on the first send error.
func (o *Overseer) runMemoryAlertTick(ctx context.Context, snap ResourceSnapshot) {
	level, reasons := o.alertThresholds.classify(snap)
	if level == PressureNone {
		return
	}

	summary := fmt.Sprintf("memory %s — %s", level, strings.Join(reasons, "; "))

	// Routing: Meta-O always; Orchestrator additionally at CRITICAL.
	targets := []Role{RoleMetaOrchestrator}
	if level >= PressureCritical {
		targets = append(targets, RoleOrchestrator)
	}

	var notified []string
	var errs []string
	for _, to := range targets {
		if err := o.alertNotifier.Notify(ctx, to, level, snap, summary); err != nil {
			errs = append(errs, string(to)+": "+err.Error())
			continue
		}
		notified = append(notified, string(to))
	}

	payload := map[string]any{
		"level":               level.String(),
		"free_physical_bytes": snap.FreePhysicalMemoryBytes,
		"free_mib":            snap.FreePhysicalMemoryBytes / MiB,
		"swap_used_fraction":  snap.SwapUsedFraction,
		"reasons":             reasons,
		"summary":             summary,
		"targets":             rolesToStrings(targets),
		"notified":            notified,
	}
	if len(errs) > 0 {
		payload["errors"] = strings.Join(errs, "; ")
	}
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role:    string(RoleOverseer),
		Kind:    "supervisor.over.memory_alert",
		Payload: payload,
	})
}

// rolesToStrings renders a role slice as strings for a trail payload.
func rolesToStrings(roles []Role) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = string(r)
	}
	return out
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
// EscalationThreshold.SwapFraction (default 0.75). Swap uses a lower threshold
// than RAM/disk/CPU because high swap is a leading indicator: if the system is
// already paging, spawning new heavy processes will cause thrashing.
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
