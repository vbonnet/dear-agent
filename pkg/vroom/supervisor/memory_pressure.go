package supervisor

import (
	"context"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// MemoryPressureMonitor and AutoResourceReaper implement the P0 memory-pressure
// directive (ce-80ca): under sustained swap/FD pressure the Overseer must
// classify the system's posture into graduated tiers and trigger remediation
// that escalates with severity, instead of the single binary escalation in
// EscalationThreshold.
//
// The graduated policy (operator directive, recorded in the bead):
//
//	warn      (swap ≥ 75%)                    → log only
//	critical  (swap ≥ 85%, or FDs ≥ 90%)      → gopls reap + stale session archive
//	emergency (swap ≥ 92%, or free < 200 MiB) → pause new worker spawns + escalate to Meta-O
//
// A higher level subsumes the remediation of every lower one: an emergency
// reaps gopls and archives sessions (the critical actions) *and* pauses spawns
// and escalates. The monitor is a pure classifier; the reaper performs the
// side-effecting actions through seams so it stays unit-testable.

// PressureLevel classifies the system's memory-pressure posture into the
// graduated tiers the AutoResourceReaper acts on. Levels are ordered so that
// a higher level subsumes every lower level's remediation.
type PressureLevel int

const (
	// PressureNone means no metric crossed even the warn threshold.
	PressureNone PressureLevel = iota
	// PressureWarn means swap is above the warn threshold — log only.
	PressureWarn
	// PressureCritical means swap or FD pressure is high enough to reap.
	PressureCritical
	// PressureEmergency means swap or free-memory is at the level where
	// spawning any new heavy process risks an out-of-memory stall.
	PressureEmergency
)

// String renders the level as a short stable token used in trail payloads.
func (l PressureLevel) String() string {
	switch l {
	case PressureNone:
		return "none"
	case PressureWarn:
		return "warn"
	case PressureCritical:
		return "critical"
	case PressureEmergency:
		return "emergency"
	default:
		return fmt.Sprintf("PressureLevel(%d)", int(l))
	}
}

// MemoryPressureThresholds is the graduated policy the MemoryPressureMonitor
// classifies against. Swap fractions are 0..1; FreeMemoryEmergencyBytes is an
// absolute floor on free physical RAM. Zero fields fall back to the defaults
// in DefaultMemoryPressureThresholds.
type MemoryPressureThresholds struct {
	// SwapWarn is the swap fraction at or above which posture is at least
	// PressureWarn. Default 0.75.
	SwapWarn float64
	// SwapCritical is the swap fraction at or above which posture is at least
	// PressureCritical. Default 0.85.
	SwapCritical float64
	// SwapEmergency is the swap fraction at or above which posture is
	// PressureEmergency. Default 0.92.
	SwapEmergency float64

	// FreeMemoryEmergencyBytes is the free-physical-RAM floor below which
	// posture is PressureEmergency regardless of swap. Default 200 MiB. A
	// snapshot reporting 0 free bytes is treated as "unknown" (the probe could
	// not measure it) and does NOT trip this floor.
	FreeMemoryEmergencyBytes uint64

	// FDCritical is the open-FD fraction at or above which posture is at least
	// PressureCritical — FD exhaustion is the documented root cause of the
	// "package errors is not in std" build failures and warrants a gopls reap
	// even when swap is calm. Default 0.90.
	FDCritical float64
}

// MiB is one mebibyte, exported for callers constructing thresholds.
const MiB = 1 << 20

// DefaultMemoryPressureThresholds is the policy used when a field is left zero.
var DefaultMemoryPressureThresholds = MemoryPressureThresholds{
	SwapWarn:                 0.75,
	SwapCritical:             0.85,
	SwapEmergency:            0.92,
	FreeMemoryEmergencyBytes: 200 * MiB,
	FDCritical:               0.90,
}

// withDefaults returns a copy of t with any zero field replaced by the default.
func (t MemoryPressureThresholds) withDefaults() MemoryPressureThresholds {
	d := DefaultMemoryPressureThresholds
	if t.SwapWarn <= 0 {
		t.SwapWarn = d.SwapWarn
	}
	if t.SwapCritical <= 0 {
		t.SwapCritical = d.SwapCritical
	}
	if t.SwapEmergency <= 0 {
		t.SwapEmergency = d.SwapEmergency
	}
	if t.FreeMemoryEmergencyBytes == 0 {
		t.FreeMemoryEmergencyBytes = d.FreeMemoryEmergencyBytes
	}
	if t.FDCritical <= 0 {
		t.FDCritical = d.FDCritical
	}
	return t
}

// MemoryPressureMonitor classifies a ResourceSnapshot into a PressureLevel.
// It is a pure function of the snapshot and thresholds — no side effects, so
// the orchestration in AutoResourceReaper can be tested independently of the
// classification policy.
type MemoryPressureMonitor struct {
	thresholds MemoryPressureThresholds
}

// NewMemoryPressureMonitor returns a monitor using the given thresholds;
// zero fields fall back to DefaultMemoryPressureThresholds.
func NewMemoryPressureMonitor(t MemoryPressureThresholds) *MemoryPressureMonitor {
	return &MemoryPressureMonitor{thresholds: t.withDefaults()}
}

// Thresholds returns the effective thresholds (after defaulting).
func (m *MemoryPressureMonitor) Thresholds() MemoryPressureThresholds { return m.thresholds }

// Classify returns the highest PressureLevel any tracked metric warrants:
// swap fraction, free physical memory, and open-FD fraction are each mapped
// to a level and the maximum wins.
func (m *MemoryPressureMonitor) Classify(snap ResourceSnapshot) PressureLevel {
	t := m.thresholds
	level := PressureNone

	// Swap-driven tiers.
	switch {
	case snap.SwapUsedFraction >= t.SwapEmergency:
		level = max(level, PressureEmergency)
	case snap.SwapUsedFraction >= t.SwapCritical:
		level = max(level, PressureCritical)
	case snap.SwapUsedFraction >= t.SwapWarn:
		level = max(level, PressureWarn)
	}

	// Free-memory floor → emergency. Zero means "unknown" (unmeasured), so it
	// must not trip the floor.
	if snap.FreePhysicalMemoryBytes > 0 && snap.FreePhysicalMemoryBytes < t.FreeMemoryEmergencyBytes {
		level = max(level, PressureEmergency)
	}

	// FD exhaustion → at least critical (a gopls reap is the remedy).
	if snap.OpenFDFraction >= t.FDCritical {
		level = max(level, PressureCritical)
	}

	return level
}

// SessionArchiver archives stale / dead AGM sessions to reclaim the memory and
// FDs they hold. The real adapter shells out to `agm session archive`; tests
// use an in-memory fake.
type SessionArchiver interface {
	ArchiveStaleSessions(ctx context.Context) (ArchiveResult, error)
}

// ArchiveResult reports the outcome of one stale-session archive pass.
type ArchiveResult struct {
	Found    int
	Archived int
	Failed   int
}

// SpawnGate controls whether the Orchestrator may spawn new worker sessions.
// Under emergency pressure the reaper pauses spawns so the in-flight workers
// can drain memory before any new heavy process is admitted.
type SpawnGate interface {
	PauseSpawns(ctx context.Context, reason string) error
	ResumeSpawns(ctx context.Context) error
}

// PressureEscalator notifies the Meta-Orchestrator of an emergency so a human
// directive or a roadmap re-prioritisation can follow. The real adapter sends
// an `agm send msg vroom-meta-orchestrator` with critical priority.
type PressureEscalator interface {
	Escalate(ctx context.Context, level PressureLevel, snap ResourceSnapshot) error
}

// AutoResourceReaper performs the graduated remediation for a PressureLevel.
// Every seam is optional: a nil seam means that action is skipped (and noted
// in the outcome), so the reaper degrades gracefully when only some adapters
// are wired.
type AutoResourceReaper struct {
	trail     decisiontrail.Trail
	reclaimer ResourceReclaimer // gopls / orphan reap (critical+)
	archiver  SessionArchiver   // stale session archive (critical+)
	gate      SpawnGate         // pause new worker spawns (emergency)
	escalator PressureEscalator // escalate to Meta-O (emergency)
}

// AutoResourceReaperOption configures an AutoResourceReaper.
type AutoResourceReaperOption func(*AutoResourceReaper)

// WithReapReclaimer wires the gopls/orphan reclaimer (critical-tier action).
func WithReapReclaimer(r ResourceReclaimer) AutoResourceReaperOption {
	return func(a *AutoResourceReaper) { a.reclaimer = r }
}

// WithSessionArchiver wires the stale-session archiver (critical-tier action).
func WithSessionArchiver(s SessionArchiver) AutoResourceReaperOption {
	return func(a *AutoResourceReaper) { a.archiver = s }
}

// WithSpawnGate wires the spawn gate (emergency-tier action).
func WithSpawnGate(g SpawnGate) AutoResourceReaperOption {
	return func(a *AutoResourceReaper) { a.gate = g }
}

// WithPressureEscalator wires the Meta-O escalator (emergency-tier action).
func WithPressureEscalator(e PressureEscalator) AutoResourceReaperOption {
	return func(a *AutoResourceReaper) { a.escalator = e }
}

// NewAutoResourceReaper constructs a reaper. The trail is required (the reaper
// records every action it takes); all remediation seams are optional.
func NewAutoResourceReaper(trail decisiontrail.Trail, opts ...AutoResourceReaperOption) (*AutoResourceReaper, error) {
	if trail == nil {
		return nil, fmt.Errorf("supervisor: AutoResourceReaper requires a Trail")
	}
	a := &AutoResourceReaper{trail: trail}
	for _, o := range opts {
		o(a)
	}
	return a, nil
}

// ReapOutcome records which graduated actions ran during one React call.
type ReapOutcome struct {
	Level        PressureLevel
	Reclaimed    *ReclaimResult // non-nil if the gopls reap ran
	Archived     *ArchiveResult // non-nil if the session archive ran
	SpawnsPaused bool           // true if PauseSpawns succeeded
	Escalated    bool           // true if Escalate succeeded
	// Errors collects best-effort failures; a failing seam never aborts the
	// remaining actions (under pressure, doing *some* remediation beats none).
	Errors []string
}

// React applies the graduated remediation for level against snap and returns
// what it did. PressureNone is a no-op. Each tier's record is appended to the
// trail so operators can see the system reacting.
func (a *AutoResourceReaper) React(ctx context.Context, level PressureLevel, snap ResourceSnapshot) ReapOutcome {
	out := ReapOutcome{Level: level}
	if level == PressureNone {
		return out
	}

	// Critical and above: reap gopls orphans and archive stale sessions.
	if level >= PressureCritical {
		a.runReclaim(ctx, &out)
		a.runArchive(ctx, &out)
	}

	// Emergency only: pause new spawns and escalate to the Meta-Orchestrator.
	if level >= PressureEmergency {
		a.runPauseSpawns(ctx, level, &out)
		a.runEscalate(ctx, level, snap, &out)
	}

	a.recordReaction(ctx, snap, out)
	return out
}

func (a *AutoResourceReaper) runReclaim(ctx context.Context, out *ReapOutcome) {
	if a.reclaimer == nil {
		return
	}
	res, err := a.reclaimer.Reclaim(ctx)
	if err != nil {
		out.Errors = append(out.Errors, "reclaim: "+err.Error())
	}
	out.Reclaimed = &res
}

func (a *AutoResourceReaper) runArchive(ctx context.Context, out *ReapOutcome) {
	if a.archiver == nil {
		return
	}
	res, err := a.archiver.ArchiveStaleSessions(ctx)
	if err != nil {
		out.Errors = append(out.Errors, "archive: "+err.Error())
	}
	out.Archived = &res
}

func (a *AutoResourceReaper) runPauseSpawns(ctx context.Context, level PressureLevel, out *ReapOutcome) {
	if a.gate == nil {
		return
	}
	reason := fmt.Sprintf("memory pressure %s", level)
	if err := a.gate.PauseSpawns(ctx, reason); err != nil {
		out.Errors = append(out.Errors, "pause_spawns: "+err.Error())
		return
	}
	out.SpawnsPaused = true
}

func (a *AutoResourceReaper) runEscalate(ctx context.Context, level PressureLevel, snap ResourceSnapshot, out *ReapOutcome) {
	if a.escalator == nil {
		return
	}
	if err := a.escalator.Escalate(ctx, level, snap); err != nil {
		out.Errors = append(out.Errors, "escalate: "+err.Error())
		return
	}
	out.Escalated = true
}

// recordReaction appends one trail record summarising the reaction.
func (a *AutoResourceReaper) recordReaction(ctx context.Context, snap ResourceSnapshot, out ReapOutcome) {
	payload := map[string]any{
		"level":               out.Level.String(),
		"swap_used_fraction":  snap.SwapUsedFraction,
		"free_physical_bytes": snap.FreePhysicalMemoryBytes,
		"open_fd_fraction":    snap.OpenFDFraction,
		"gopls_processes":     snap.GoplsProcesses,
		"spawns_paused":       out.SpawnsPaused,
		"escalated":           out.Escalated,
	}
	if out.Reclaimed != nil {
		payload["orphans_killed"] = out.Reclaimed.OrphansKilled
	}
	if out.Archived != nil {
		payload["sessions_archived"] = out.Archived.Archived
	}
	if len(out.Errors) > 0 {
		payload["errors"] = strings.Join(out.Errors, "; ")
	}
	_ = a.trail.Append(ctx, decisiontrail.Record{
		Role:    string(RoleOverseer),
		Kind:    "supervisor.over.memory_pressure",
		Payload: payload,
	})
}

// ResourceIncident captures a memory-pressure event for a DEAR retro
// (Define → Execute → Audit → Retro, per CONTEXT.md). Before/After bracket the
// remediation so the Audit phase can report whether pressure actually dropped.
type ResourceIncident struct {
	Level   PressureLevel
	Before  ResourceSnapshot
	After   ResourceSnapshot
	Outcome ReapOutcome
}

// pressureDropped reports whether the post-remediation snapshot shows swap or
// FD pressure easing relative to before.
func (inc ResourceIncident) pressureDropped() bool {
	return inc.After.SwapUsedFraction < inc.Before.SwapUsedFraction ||
		inc.After.OpenFDFraction < inc.Before.OpenFDFraction ||
		inc.After.GoplsProcesses < inc.Before.GoplsProcesses
}

// ResourceIncidentRetro renders a ResourceIncident as a DEAR-formatted trail
// record. The four phases map to payload keys so an Auditor (or a human
// reading the trail) gets a self-contained narrative of one incident:
//
//	Define  — what tripped the level and the metric values
//	Execute — which graduated actions the reaper ran
//	Audit   — did the remediation move the metrics
//	Retro   — the lesson / recommended follow-up
//
// Findings flow back to the roadmap via the Meta-Orchestrator (CONTEXT.md
// §DEAR), so the record kind is stable and greppable.
func ResourceIncidentRetro(inc ResourceIncident) decisiontrail.Record {
	dropped := inc.pressureDropped()

	define := fmt.Sprintf(
		"memory pressure reached %s: swap=%.0f%% free=%dMiB fds=%.0f%% gopls=%d",
		inc.Level,
		inc.Before.SwapUsedFraction*100,
		inc.Before.FreePhysicalMemoryBytes/MiB,
		inc.Before.OpenFDFraction*100,
		inc.Before.GoplsProcesses,
	)

	execute := describeActions(inc.Outcome)

	audit := fmt.Sprintf(
		"after remediation: swap=%.0f%% free=%dMiB fds=%.0f%% gopls=%d — pressure %s",
		inc.After.SwapUsedFraction*100,
		inc.After.FreePhysicalMemoryBytes/MiB,
		inc.After.OpenFDFraction*100,
		inc.After.GoplsProcesses,
		map[bool]string{true: "eased", false: "did NOT ease"}[dropped],
	)

	retro := retroLesson(inc, dropped)

	return decisiontrail.Record{
		Role: string(RoleOverseer),
		Kind: "supervisor.over.dear_retro",
		Payload: map[string]any{
			"incident": "memory_pressure",
			"level":    inc.Level.String(),
			"define":   define,
			"execute":  execute,
			"audit":    audit,
			"retro":    retro,
			"resolved": dropped,
		},
	}
}

// describeActions summarises the actions taken in a ReapOutcome as a phrase
// for the Execute phase.
func describeActions(out ReapOutcome) string {
	var parts []string
	if out.Reclaimed != nil {
		parts = append(parts, fmt.Sprintf("reaped %d orphan(s)", out.Reclaimed.OrphansKilled))
	}
	if out.Archived != nil {
		parts = append(parts, fmt.Sprintf("archived %d stale session(s)", out.Archived.Archived))
	}
	if out.SpawnsPaused {
		parts = append(parts, "paused new worker spawns")
	}
	if out.Escalated {
		parts = append(parts, "escalated to Meta-Orchestrator")
	}
	if len(parts) == 0 {
		return "no remediation seams wired — log only"
	}
	joined := strings.Join(parts, ", ")
	if len(out.Errors) > 0 {
		joined += " (errors: " + strings.Join(out.Errors, "; ") + ")"
	}
	return joined
}

// retroLesson derives the Retro-phase recommendation from the incident.
func retroLesson(inc ResourceIncident, dropped bool) string {
	switch {
	case inc.Level >= PressureEmergency && !dropped:
		return "emergency remediation did not relieve pressure; recommend raising swap pool or lowering AGM_MAX_WORKERS — escalate to roadmap"
	case inc.Level >= PressureEmergency:
		return "emergency remediation relieved pressure; recommend tracking spawn-pause frequency — chronic pauses mean the worker cap is too high for available RAM"
	case inc.Level == PressureCritical && !dropped:
		return "gopls reap / session archive did not relieve pressure; the leak source is elsewhere — audit FD holders beyond gopls"
	case inc.Level == PressureCritical:
		return "critical remediation relieved pressure; gopls/session accumulation is the recurring driver — keep the reaper wired"
	default:
		return "warn-level pressure observed; no action needed yet — monitor for escalation"
	}
}
