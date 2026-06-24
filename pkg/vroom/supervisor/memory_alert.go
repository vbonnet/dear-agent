package supervisor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Memory-pressure alerting (ce-6jl5).
//
// The 2026-06-18 crisis was missed because the Overseer tick loop never
// surfaced two leading indicators of memory exhaustion: how little physical
// RAM remained free, and how hard the system was already swapping. Both
// signals existed in the ResourceSnapshot (FreePhysicalMemoryBytes and
// SwapUsedFraction) but no tick step classified them into a severity and
// routed an alert to a supervisor that could act.
//
// This file adds that step. It is deliberately separate from the graduated
// AutoResourceReaper (ce-80ca): the reaper *remediates* (reaps gopls, archives
// sessions, pauses spawns) on swap/FD pressure with its own thresholds, whereas
// this is a thin *alerting* probe with the operator-directed thresholds from
// the retro and an explicit Meta-O / Orchestrator routing fan-out. Keeping them
// distinct lets the alert thresholds (a free-RAM floor, a lower swap warn) be
// tuned without perturbing the reaper's remediation policy.
//
// The graduated policy (operator directive, recorded in the bead):
//
//	free < 500 MiB  → WARN       free < 200 MiB  → CRITICAL
//	swap > 50%      → WARN       swap > 75%      → CRITICAL
//
// Routing: every alert escalates to the Meta-Orchestrator; a CRITICAL alert
// also escalates to the Orchestrator (which owns the spawn queue and can stop
// admitting new heavy workers). The level reuses PressureLevel so the warn /
// critical tokens stay consistent across the resource-monitoring code; only
// PressureWarn and PressureCritical are produced here.

// MemoryAlertThresholds is the operator-directed policy the memory-alert tick
// classifies a ResourceSnapshot against. Free-RAM floors are absolute byte
// counts (a fraction hides how little headroom remains on a large-RAM box);
// swap is a 0..1 fraction. Zero fields fall back to DefaultMemoryAlertThresholds.
type MemoryAlertThresholds struct {
	// FreeWarnBytes is the free-physical-RAM floor below which the alert is at
	// least WARN. Default 500 MiB.
	FreeWarnBytes uint64
	// FreeCriticalBytes is the free-physical-RAM floor below which the alert is
	// CRITICAL. Default 200 MiB.
	FreeCriticalBytes uint64

	// SwapWarn is the swap fraction above which the alert is at least WARN.
	// Default 0.50.
	SwapWarn float64
	// SwapCritical is the swap fraction above which the alert is CRITICAL.
	// Default 0.75.
	SwapCritical float64
}

// DefaultMemoryAlertThresholds is the policy used when a field is left zero.
// These are the thresholds the 2026-06-18 retro prescribed for the signals
// that were invisible during the crisis.
var DefaultMemoryAlertThresholds = MemoryAlertThresholds{
	FreeWarnBytes:     500 * MiB,
	FreeCriticalBytes: 200 * MiB,
	SwapWarn:          0.50,
	SwapCritical:      0.75,
}

// withDefaults returns a copy of t with any zero field replaced by the default.
func (t MemoryAlertThresholds) withDefaults() MemoryAlertThresholds {
	d := DefaultMemoryAlertThresholds
	if t.FreeWarnBytes == 0 {
		t.FreeWarnBytes = d.FreeWarnBytes
	}
	if t.FreeCriticalBytes == 0 {
		t.FreeCriticalBytes = d.FreeCriticalBytes
	}
	if t.SwapWarn <= 0 {
		t.SwapWarn = d.SwapWarn
	}
	if t.SwapCritical <= 0 {
		t.SwapCritical = d.SwapCritical
	}
	return t
}

// classify maps a snapshot to the highest memory-alert level its free-RAM and
// swap metrics warrant, alongside a per-metric reason list explaining what
// tripped. PressureNone with a nil reason list means the snapshot is healthy.
//
// A FreePhysicalMemoryBytes of 0 means the probe could not measure free RAM
// ("unknown") and never trips the floor — only a positive-but-low reading does,
// mirroring MemoryPressureMonitor.Classify. Free uses strict < and swap strict
// > so the thresholds read exactly as the retro stated them ("free < 500 MiB",
// "swap > 50%").
func (t MemoryAlertThresholds) classify(snap ResourceSnapshot) (PressureLevel, []string) {
	t = t.withDefaults()
	level := PressureNone
	var reasons []string

	if snap.FreePhysicalMemoryBytes > 0 {
		freeMiB := snap.FreePhysicalMemoryBytes / MiB
		switch {
		case snap.FreePhysicalMemoryBytes < t.FreeCriticalBytes:
			level = max(level, PressureCritical)
			reasons = append(reasons, fmt.Sprintf("free %dMiB < %dMiB (critical)", freeMiB, t.FreeCriticalBytes/MiB))
		case snap.FreePhysicalMemoryBytes < t.FreeWarnBytes:
			level = max(level, PressureWarn)
			reasons = append(reasons, fmt.Sprintf("free %dMiB < %dMiB (warn)", freeMiB, t.FreeWarnBytes/MiB))
		}
	}

	switch {
	case snap.SwapUsedFraction > t.SwapCritical:
		level = max(level, PressureCritical)
		reasons = append(reasons, fmt.Sprintf("swap %.0f%% > %.0f%% (critical)", snap.SwapUsedFraction*100, t.SwapCritical*100))
	case snap.SwapUsedFraction > t.SwapWarn:
		level = max(level, PressureWarn)
		reasons = append(reasons, fmt.Sprintf("swap %.0f%% > %.0f%% (warn)", snap.SwapUsedFraction*100, t.SwapWarn*100))
	}

	return level, reasons
}

// MemoryAlertNotifier delivers one memory-pressure alert to a specific
// supervisor role. The Overseer owns the routing decision (which roles to
// notify at which level); the notifier is the dumb delivery seam. The real
// adapter (AGMMemoryAlertNotifier) shells out to `agm send msg`; tests use
// InMemoryMemoryAlertNotifier.
type MemoryAlertNotifier interface {
	Notify(ctx context.Context, to Role, level PressureLevel, snap ResourceSnapshot, summary string) error
}

// AGMMemoryAlertNotifier is the production MemoryAlertNotifier: it relays the
// alert to a supervisor's session via `agm send msg <session> --sender overseer
// --autonomous --prompt <summary>`, mirroring how AGMRecovery nudges a stale
// peer. The agm command is state-aware (it queues against the target session),
// so delivering to a transiently-busy supervisor is harmless.
type AGMMemoryAlertNotifier struct {
	// Binary is the agm executable name or path. Defaults to "agm".
	Binary string

	// SessionFor maps a supervisor Role to its AGM session name. Defaults to the
	// identity mapping (the Role string is the session name), which matches the
	// canonical mesh session names ("meta-orchestrator", "orchestrator").
	SessionFor func(Role) string

	// RunCommand runs the send command and returns its combined output. Injectable
	// so tests can observe the invocation without spawning a process. Defaults to
	// exec.CommandContext + CombinedOutput.
	RunCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Notify implements MemoryAlertNotifier.
func (n *AGMMemoryAlertNotifier) Notify(ctx context.Context, to Role, level PressureLevel, _ ResourceSnapshot, summary string) error {
	binary := n.Binary
	if binary == "" {
		binary = "agm"
	}
	session := string(to)
	if n.SessionFor != nil {
		session = n.SessionFor(to)
	}
	if session == "" {
		return fmt.Errorf("supervisor: AGMMemoryAlertNotifier has no session name for role %q", to)
	}

	run := n.RunCommand
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}

	prompt := fmt.Sprintf(
		"MEMORY ALERT (overseer, %s): %s. The host is approaching memory exhaustion; "+
			"investigate and shed load (pause spawns / reap orphans / archive sessions) as appropriate.",
		level, summary)
	args := []string{"send", "msg", session, "--sender", string(RoleOverseer), "--autonomous", "--prompt", prompt}
	out, err := run(ctx, binary, args...)
	if err != nil {
		return fmt.Errorf("supervisor: memory alert to %q failed: %w (output: %s)",
			session, err, strings.TrimSpace(string(out)))
	}
	return nil
}
