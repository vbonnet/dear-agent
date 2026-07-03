package supervisor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Disk-free + inode alerting (ce-6fel).
//
// The disk filled to 0 twice on 2026-07-03 with zero prior alert. The Overseer
// already recorded DiskUsedFraction every tick, but a fraction alone hides how
// little absolute headroom remains on a large disk (the same lesson as the
// free-RAM floor in ce-6jl5), and inode exhaustion — which fails writes while
// `df -h` still shows free space — was not measured at all.
//
// This file adds both signals, mirroring the memory-pressure alert (ce-6jl5):
// a thin *alerting* probe with operator-directed thresholds and an explicit
// Meta-O / Orchestrator routing fan-out. It deliberately does not remediate —
// the host-level cmd/disk-watchdog owns the remediation hook (reaping
// provably-merged worktree husks via `agm worktree sweep --execute`).
//
// The graduated policy (disk-retro 2026-07-03):
//
//	free < 20 GiB   → WARN       free < 5 GiB    → CRITICAL
//	inodes > 90%    → WARN       inodes > 95%    → CRITICAL
//
// Routing: every alert escalates to the Meta-Orchestrator; a CRITICAL alert
// also escalates to the Orchestrator (which owns the spawn queue and can stop
// admitting new workers — each spawn clones a worktree, the dominant disk
// writer on this host). Only PressureWarn and PressureCritical are produced.

// GiB is 2^30 bytes, the unit the disk thresholds are stated in.
const GiB = 1 << 30

// DiskAlertThresholds is the operator-directed policy the disk-alert tick
// classifies a ResourceSnapshot against. Free-space floors are absolute byte
// counts (a fraction hides how little headroom remains on a large disk);
// inodes are a 0..1 used fraction. Zero fields fall back to
// DefaultDiskAlertThresholds.
type DiskAlertThresholds struct {
	// FreeWarnBytes is the free-disk floor below which the alert is at least
	// WARN. Default 20 GiB.
	FreeWarnBytes uint64
	// FreeCriticalBytes is the free-disk floor below which the alert is
	// CRITICAL. Default 5 GiB.
	FreeCriticalBytes uint64

	// InodeWarn is the inode used fraction above which the alert is at least
	// WARN. Default 0.90.
	InodeWarn float64
	// InodeCritical is the inode used fraction above which the alert is
	// CRITICAL. Default 0.95.
	InodeCritical float64
}

// DefaultDiskAlertThresholds is the policy used when a field is left zero.
// These are the thresholds the 2026-07-03 disk retro prescribed for the
// signals that were invisible while the disk filled to 0 twice.
var DefaultDiskAlertThresholds = DiskAlertThresholds{
	FreeWarnBytes:     20 * GiB,
	FreeCriticalBytes: 5 * GiB,
	InodeWarn:         0.90,
	InodeCritical:     0.95,
}

// withDefaults returns a copy of t with any zero field replaced by the default.
func (t DiskAlertThresholds) withDefaults() DiskAlertThresholds {
	d := DefaultDiskAlertThresholds
	if t.FreeWarnBytes == 0 {
		t.FreeWarnBytes = d.FreeWarnBytes
	}
	if t.FreeCriticalBytes == 0 {
		t.FreeCriticalBytes = d.FreeCriticalBytes
	}
	if t.InodeWarn <= 0 {
		t.InodeWarn = d.InodeWarn
	}
	if t.InodeCritical <= 0 {
		t.InodeCritical = d.InodeCritical
	}
	return t
}

// Classify maps a snapshot to the highest disk-alert level its free-space and
// inode metrics warrant, alongside a per-metric reason list explaining what
// tripped. PressureNone with a nil reason list means the snapshot is healthy.
//
// A snapshot where DiskFreeBytes and DiskUsedFraction are both zero means the
// probe could not stat the filesystem ("unknown") and never trips the floor,
// mirroring the free-RAM convention in MemoryAlertThresholds.classify. A
// *measured* zero still alarms — free hitting 0 is the exact ce-6fel crash
// state — and is distinguishable because a successful statfs always reports a
// nonzero used fraction. Free uses strict < and inodes strict > so the
// thresholds read exactly as the retro stated them ("free < 20 GiB",
// "inodes > 90%").
//
// Exported — unlike the memory-alert classifier — because cmd/disk-watchdog
// reuses this exact logic so the launchd watchdog and the Overseer tick can
// never disagree about what "low disk" means.
func (t DiskAlertThresholds) Classify(snap ResourceSnapshot) (PressureLevel, []string) {
	t = t.withDefaults()
	level := PressureNone
	var reasons []string

	if snap.DiskFreeBytes > 0 || snap.DiskUsedFraction > 0 {
		freeGiB := float64(snap.DiskFreeBytes) / GiB
		switch {
		case snap.DiskFreeBytes < t.FreeCriticalBytes:
			level = max(level, PressureCritical)
			reasons = append(reasons, fmt.Sprintf("disk free %.1fGiB < %.0fGiB (critical)", freeGiB, float64(t.FreeCriticalBytes)/GiB))
		case snap.DiskFreeBytes < t.FreeWarnBytes:
			level = max(level, PressureWarn)
			reasons = append(reasons, fmt.Sprintf("disk free %.1fGiB < %.0fGiB (warn)", freeGiB, float64(t.FreeWarnBytes)/GiB))
		}
	}

	switch {
	case snap.InodeUsedFraction > t.InodeCritical:
		level = max(level, PressureCritical)
		reasons = append(reasons, fmt.Sprintf("inodes %.0f%% > %.0f%% (critical)", snap.InodeUsedFraction*100, t.InodeCritical*100))
	case snap.InodeUsedFraction > t.InodeWarn:
		level = max(level, PressureWarn)
		reasons = append(reasons, fmt.Sprintf("inodes %.0f%% > %.0f%% (warn)", snap.InodeUsedFraction*100, t.InodeWarn*100))
	}

	return level, reasons
}

// DiskAlertNotifier delivers one disk-pressure alert to a specific supervisor
// role. The Overseer owns the routing decision (which roles to notify at which
// level); the notifier is the dumb delivery seam. The real adapter
// (AGMDiskAlertNotifier) shells out to `agm send msg`; tests use
// InMemoryDiskAlertNotifier.
type DiskAlertNotifier interface {
	Notify(ctx context.Context, to Role, level PressureLevel, snap ResourceSnapshot, summary string) error
}

// AGMDiskAlertNotifier is the production DiskAlertNotifier: it relays the
// alert to a supervisor's session via `agm send msg <session> --sender
// overseer --autonomous --prompt <summary>`, mirroring AGMMemoryAlertNotifier.
// The agm command is state-aware (it queues against the target session), so
// delivering to a transiently-busy supervisor is harmless.
type AGMDiskAlertNotifier struct {
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

// Notify implements DiskAlertNotifier.
func (n *AGMDiskAlertNotifier) Notify(ctx context.Context, to Role, level PressureLevel, _ ResourceSnapshot, summary string) error {
	binary := n.Binary
	if binary == "" {
		binary = "agm"
	}
	session := string(to)
	if n.SessionFor != nil {
		session = n.SessionFor(to)
	}
	if session == "" {
		return fmt.Errorf("supervisor: AGMDiskAlertNotifier has no session name for role %q", to)
	}

	run := n.RunCommand
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}

	prompt := fmt.Sprintf(
		"DISK ALERT (overseer, %s): %s. The host is approaching disk exhaustion; "+
			"free space before writes start failing (reap merged worktree husks via "+
			"'agm worktree sweep --execute', archive stopped sessions, pause spawns).",
		level, summary)
	args := []string{"send", "msg", session, "--sender", string(RoleOverseer), "--autonomous", "--prompt", prompt}
	out, err := run(ctx, binary, args...)
	if err != nil {
		return fmt.Errorf("supervisor: disk alert to %q failed: %w (output: %s)",
			session, err, strings.TrimSpace(string(out)))
	}
	return nil
}
