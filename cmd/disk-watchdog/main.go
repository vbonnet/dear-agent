// Command disk-watchdog samples disk free-space + inode pressure, alarms when
// a threshold is breached, logs the alarm to the VROOM decision trail, and
// remediates by reaping provably-merged worktree husks.
//
// It is the host-level half of the ce-6fel fix: on 2026-07-03 the disk filled
// to 0 twice with zero prior alert. The Overseer's tick now classifies the
// same thresholds in-process (supervisor.DiskAlertThresholds, routed to
// Meta-O/Orchestrator), but the mesh only alarms while it is running — this
// launchd-driven watchdog is the independent backstop that fires even when
// every supervisor is down, which is exactly the state a full disk causes.
//
// Detection and the in-process alert share one classifier
// (supervisor.DiskAlertThresholds.Classify), so the watchdog and the Overseer
// can never disagree about what "low disk" means. Thresholds (disk-retro
// 2026-07-03 defaults):
//
//	free   : < 20 GiB WARN, < 5 GiB CRITICAL
//	inodes : > 90% WARN, > 95% CRITICAL
//
// Remediation reuses sanctioned safe hooks: first `agm sandbox gc --reap`,
// then `agm worktree sweep --execute`. The sandbox GC re-verifies live-session,
// process, mount, path, and age gates; the worktree sweep removes only
// provably-MERGED, clean worktrees. No new destructive cleanup is invented here.
//
// Usage:
//
//	disk-watchdog                        # human-readable report; remediate if breached
//	disk-watchdog --json                 # JSON report to stdout
//	disk-watchdog --dry-run              # detect + log, but do not sweep anything
//	disk-watchdog --path /               # filesystem to measure (default "/")
//	disk-watchdog --free-warn-gb 40      # override the free-space WARN floor
//	disk-watchdog --free-critical-gb 10  # override the free-space CRITICAL floor
//	disk-watchdog --inode-warn 0.85      # override the inode WARN ceiling
//	disk-watchdog --inode-critical 0.92  # override the inode CRITICAL ceiling
//	disk-watchdog --brake /path/brake.json  # admission-brake location
//	disk-watchdog --brake-ttl 45m        # how long an engaged brake blocks spawns
//
// Beyond alarming and remediating, the watchdog drives the cross-process
// admission brake (pkg/vroom/admission): when its own remediation fails, or
// when it cannot take a snapshot at all, it latches the brake and every spawn
// path refuses new work until a healthy tick releases it or the TTL expires.
// That is the ce-93lw.18 fix — on 2026-07-18 this watchdog was in ALARM with
// `agm worktree sweep --execute: signal: killed` in every remediation slot, and
// nothing consumed that fact, so the mesh kept spawning into a wedged host.
//
// Exit codes: 0 = within limits; 1 = at least one threshold breached (an alarm
// fired); 2 = usage/runtime error. Brake I/O never changes the exit code.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/admission"
	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func main() {
	code, err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "disk-watchdog:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

// probeTimeout bounds the statfs snapshot; sweepTimeout bounds the worktree
// sweep, which shells out to gh per worktree and can legitimately take minutes
// on a husk-heavy machine; trailTimeout bounds the alarm append, which can
// stall when the disk being alarmed about is already exhausted.
const (
	probeTimeout = 15 * time.Second
	sweepTimeout = 10 * time.Minute
	trailTimeout = 30 * time.Second
)

type config struct {
	jsonOutput bool
	dryRun     bool
	path       string
	agmBin     string
	trailPath  string
	brakePath  string
	brakeTTL   time.Duration
	thresholds supervisor.DiskAlertThresholds

	// runCommand is the exec seam for remediation; nil = real exec. Injectable
	// so tests can observe the sweep invocation without spawning a process.
	runCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func run(args []string, out io.Writer) (int, error) {
	fs := flag.NewFlagSet("disk-watchdog", flag.ContinueOnError)
	fs.SetOutput(out)
	cfg := config{}
	var freeWarnGB, freeCriticalGB float64
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit JSON instead of a human-readable report")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "detect and log, but do not reap any worktrees")
	fs.StringVar(&cfg.path, "path", "/", "filesystem path to measure")
	fs.StringVar(&cfg.agmBin, "agm", "agm", "path to the agm binary used for worktree-sweep remediation")
	fs.StringVar(&cfg.trailPath, "trail", defaultTrailPath(), "decision-trail JSONL path for alarm records")
	fs.StringVar(&cfg.brakePath, "brake", admission.DefaultPath(),
		"admission-brake path; engaged when remediation fails, released on a healthy tick")
	fs.DurationVar(&cfg.brakeTTL, "brake-ttl", admission.DefaultTTL,
		"how long an engaged admission brake blocks spawns before expiring on its own")
	fs.Float64Var(&freeWarnGB, "free-warn-gb", float64(supervisor.DefaultDiskAlertThresholds.FreeWarnBytes)/supervisor.GiB,
		"alarm WARN when free disk space (GiB) falls below this value")
	fs.Float64Var(&freeCriticalGB, "free-critical-gb", float64(supervisor.DefaultDiskAlertThresholds.FreeCriticalBytes)/supervisor.GiB,
		"alarm CRITICAL when free disk space (GiB) falls below this value")
	fs.Float64Var(&cfg.thresholds.InodeWarn, "inode-warn", supervisor.DefaultDiskAlertThresholds.InodeWarn,
		"alarm WARN when inode usage exceeds this fraction [0,1]")
	fs.Float64Var(&cfg.thresholds.InodeCritical, "inode-critical", supervisor.DefaultDiskAlertThresholds.InodeCritical,
		"alarm CRITICAL when inode usage exceeds this fraction [0,1]")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	cfg.thresholds.FreeWarnBytes = uint64(freeWarnGB * supervisor.GiB)
	cfg.thresholds.FreeCriticalBytes = uint64(freeCriticalGB * supervisor.GiB)

	snap, err := takeSnapshot(cfg.path)
	if err != nil {
		// A watchdog that cannot measure the thing it guards is itself a
		// saturation signal (ce-93lw.18 (c)). Latch the brake before returning,
		// so spawns stop while we are blind rather than continuing on the last
		// reading nobody has.
		applyBrake(cfg, true, fmt.Sprintf("cannot read a disk snapshot for %s: %v", cfg.path, err))
		return 2, fmt.Errorf("snapshot: %w", err)
	}

	level, reasons := cfg.thresholds.Classify(snap)
	breached := level != supervisor.PressureNone

	var remediation *sweepResult
	if breached && !cfg.dryRun {
		r, rerr := sweepMergedWorktrees(context.Background(), cfg)
		if rerr != nil {
			// Remediation failure must not hide the alarm; record it and keep going.
			r = &sweepResult{Error: rerr.Error()}
		}
		remediation = r
	}

	updateAdmissionBrake(cfg, breached, remediation)

	// Logging the alarm to the trail is best-effort: a trail write failure is a
	// warning, never a reason to suppress the breach exit code — and on a truly
	// full disk the append will fail while the alarm must still exit 1. The
	// write is timeout-bounded because I/O on an exhausted disk can stall.
	if breached {
		logCtx, logCancel := context.WithTimeout(context.Background(), trailTimeout)
		lerr := logAlarm(logCtx, cfg, snap, level, reasons, remediation)
		logCancel()
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "disk-watchdog: warning: trail append failed: %v\n", lerr)
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(out, snap, level, reasons, remediation, cfg); err != nil {
			return 2, err
		}
	} else {
		emitReport(out, snap, level, reasons, remediation, cfg)
	}

	if breached {
		return 1, nil
	}
	return 0, nil
}

// takeSnapshot samples the filesystem under a bounded context; defer cancel()
// in its own scope guarantees cleanup on every return path.
func takeSnapshot(path string) (supervisor.ResourceSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	probe := supervisor.NewSysResourceProbe()
	probe.DiskPath = path
	return probe.Snapshot(ctx)
}

func defaultTrailPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/vroom/trail.jsonl"
	}
	return filepath.Join(home, ".agm", "vroom", "trail.jsonl")
}

// sweepResult is the parsed JSON contract of `agm worktree sweep -o json`
// (a subset; field names mirror agm/internal/ops.SweepResult).
type sweepResult struct {
	SandboxGC *sandboxGCSummary `json:"sandbox_gc,omitempty"`
	Removed   []string          `json:"removed"`
	Failed    map[string]string `json:"failed,omitempty"`
	Error     string            `json:"error,omitempty"`
}

type sandboxGCSummary struct {
	Scanned int    `json:"scanned"`
	Reaped  int    `json:"reaped"`
	Kept    int    `json:"kept"`
	Errors  int    `json:"errors"`
	Error   string `json:"error,omitempty"`
}

// sweepMergedWorktrees delegates remediation to the canonical worktree sweep,
// with --execute. The sweep only ever removes worktrees that are provably
// MERGED and clean (ancestor-of-base or an authoritative gh PR state of
// MERGED); ACTIVE, DIRTY, ORPHANED, and unpushed work are never touched — the
// safety invariant that makes this hook usable from an unattended watchdog.
func sweepMergedWorktrees(ctx context.Context, cfg config) (*sweepResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sweepTimeout)
	defer cancel()

	run := cfg.runCommand
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			// The sweep shells out to git/gh; with no TTY a credential prompt
			// would hang the launchd job until the timeout, so forbid prompts.
			cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			// Capture stdout only; the sweep logs progress to stderr, which we
			// intentionally drop so JSON parsing sees clean output.
			return cmd.Output()
		}
	}

	result := &sweepResult{}
	gcOut, gcErr := run(ctx, cfg.agmBin, "sandbox", "gc", "--reap", "--json")
	if gcErr != nil {
		result.SandboxGC = &sandboxGCSummary{Error: gcErr.Error()}
	} else if err := json.Unmarshal(gcOut, &result.SandboxGC); err != nil {
		result.SandboxGC = &sandboxGCSummary{Error: fmt.Sprintf("parse sandbox gc output: %v", err)}
	}

	out, err := run(ctx, cfg.agmBin, "worktree", "sweep", "--execute", "-o", "json")
	if err != nil {
		result.Error = fmt.Sprintf("%s worktree sweep --execute: %v", cfg.agmBin, err)
		return result, fmt.Errorf("%s worktree sweep --execute: %w", cfg.agmBin, err)
	}
	if err := json.Unmarshal(out, result); err != nil {
		result.Error = fmt.Sprintf("parse worktree sweep output: %v", err)
		return result, fmt.Errorf("parse worktree sweep output: %w", err)
	}
	if result.SandboxGC != nil && result.SandboxGC.Error != "" {
		sboxErr := fmt.Sprintf("%s sandbox gc --reap: %s", cfg.agmBin, result.SandboxGC.Error)
		if result.Error != "" {
			result.Error = fmt.Sprintf("%s; %s", sboxErr, result.Error)
		} else {
			result.Error = sboxErr
		}
	}
	return result, nil
}

// brakeSource identifies this watchdog in the admission-brake record.
const brakeSource = "disk-watchdog"

// brakeDecision is what a tick concluded about the admission brake. Pure, so
// the policy can be tested without touching the filesystem.
type brakeDecision struct {
	// Engage latches the brake; Release clears it. Both false means leave any
	// existing brake alone and let its TTL run.
	Engage  bool
	Release bool
	Reason  string
}

// decideBrake maps a tick outcome onto a brake transition.
//
// The case that matters is row one. On 2026-07-18 this watchdog ticked every
// five minutes with root at 96.2% used and
// `agm worktree sweep --execute: signal: killed` in every remediation slot —
// the remediation path was being killed by the exhaustion it existed to
// relieve — and the mesh kept spawning because that fact had no consumer.
//
// A breached tick whose remediation *succeeded* deliberately leaves an existing
// brake alone rather than clearing it: one successful sweep under an active
// alarm is not evidence the host is healthy. Only an unbreached tick releases.
func decideBrake(breached bool, rem *sweepResult) brakeDecision {
	switch {
	case !breached:
		return brakeDecision{Release: true}
	case rem != nil && rem.Error != "":
		return brakeDecision{
			Engage: true,
			Reason: fmt.Sprintf("worktree-sweep remediation failed: %s", rem.Error),
		}
	default:
		return brakeDecision{}
	}
}

// updateAdmissionBrake applies the tick's brake decision.
func updateAdmissionBrake(cfg config, breached bool, rem *sweepResult) {
	d := decideBrake(breached, rem)
	switch {
	case d.Engage:
		applyBrake(cfg, true, d.Reason)
	case d.Release:
		applyBrake(cfg, false, "")
	}
}

// applyBrake engages or releases the brake, honouring --dry-run.
//
// Brake I/O is best-effort in exactly the same way the trail append is: a write
// failure is a warning on stderr and never changes the exit code, because the
// alarm itself must still be reported. On a truly full disk the brake write is
// one of the operations most likely to fail, and swallowing the alarm to report
// that would be the wrong trade.
func applyBrake(cfg config, engage bool, reason string) {
	if cfg.dryRun || cfg.brakePath == "" {
		return
	}
	if engage {
		if err := admission.Engage(cfg.brakePath, brakeSource, reason, cfg.brakeTTL); err != nil {
			fmt.Fprintf(os.Stderr, "disk-watchdog: warning: could not engage admission brake: %v\n", err)
		}
		return
	}
	// Scoped to this source so a healthy disk tick cannot clear a brake
	// vroom-governor engaged because its own probes had gone unreadable.
	if err := admission.ReleaseBySource(cfg.brakePath, brakeSource); err != nil {
		fmt.Fprintf(os.Stderr, "disk-watchdog: warning: could not release admission brake: %v\n", err)
	}
}

// logAlarm appends one watchdog.disk.alarm record to the decision trail.
func logAlarm(ctx context.Context, cfg config, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult) error {
	trail, err := decisiontrail.OpenJSONL(cfg.trailPath)
	if err != nil {
		return err
	}
	defer trail.Close()

	payload := map[string]any{
		"level":               level.String(),
		"path":                cfg.path,
		"disk_free_bytes":     snap.DiskFreeBytes,
		"disk_free_gib":       float64(snap.DiskFreeBytes) / supervisor.GiB,
		"disk_used_fraction":  snap.DiskUsedFraction,
		"inode_used_fraction": snap.InodeUsedFraction,
		"reasons":             reasons,
		"dry_run":             cfg.dryRun,
		"thresholds": map[string]any{
			"free_warn_bytes":     cfg.thresholds.FreeWarnBytes,
			"free_critical_bytes": cfg.thresholds.FreeCriticalBytes,
			"inode_warn":          cfg.thresholds.InodeWarn,
			"inode_critical":      cfg.thresholds.InodeCritical,
		},
	}
	if rem != nil {
		remediation := map[string]any{
			"worktrees_removed": len(rem.Removed),
			"removed":           rem.Removed,
		}
		if rem.SandboxGC != nil {
			remediation["sandbox_gc"] = rem.SandboxGC
		}
		if len(rem.Failed) > 0 {
			remediation["failed"] = rem.Failed
		}
		if rem.Error != "" {
			remediation["error"] = rem.Error
		}
		payload["remediation"] = remediation
	}

	return trail.Append(ctx, decisiontrail.Record{
		Role:    "watchdog",
		Kind:    "watchdog.disk.alarm",
		Payload: payload,
	})
}

func emitJSON(out io.Writer, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, cfg config) error {
	type report struct {
		Level             string                         `json:"level"`
		DiskFreeBytes     uint64                         `json:"disk_free_bytes"`
		DiskFreeGiB       float64                        `json:"disk_free_gib"`
		DiskUsedFraction  float64                        `json:"disk_used_fraction"`
		InodeUsedFraction float64                        `json:"inode_used_fraction"`
		Thresholds        supervisor.DiskAlertThresholds `json:"thresholds"`
		Reasons           []string                       `json:"reasons"`
		Remediation       *sweepResult                   `json:"remediation,omitempty"`
		OK                bool                           `json:"ok"`
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report{
		Level:             level.String(),
		DiskFreeBytes:     snap.DiskFreeBytes,
		DiskFreeGiB:       float64(snap.DiskFreeBytes) / supervisor.GiB,
		DiskUsedFraction:  snap.DiskUsedFraction,
		InodeUsedFraction: snap.InodeUsedFraction,
		Thresholds:        cfg.thresholds,
		Reasons:           reasons,
		Remediation:       rem,
		OK:                level == supervisor.PressureNone,
	})
}

func emitReport(out io.Writer, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, cfg config) {
	fmt.Fprintln(out, "disk-watchdog report")
	fmt.Fprintf(out, "  path        : %s\n", cfg.path)
	fmt.Fprintf(out, "  disk free   : %.1f GiB  [warn < %.0f GiB, critical < %.0f GiB]\n",
		float64(snap.DiskFreeBytes)/supervisor.GiB,
		float64(cfg.thresholds.FreeWarnBytes)/supervisor.GiB,
		float64(cfg.thresholds.FreeCriticalBytes)/supervisor.GiB)
	fmt.Fprintf(out, "  disk used   : %.1f%%\n", snap.DiskUsedFraction*100)
	fmt.Fprintf(out, "  inode usage : %.1f%%  [warn > %.0f%%, critical > %.0f%%]\n",
		snap.InodeUsedFraction*100, cfg.thresholds.InodeWarn*100, cfg.thresholds.InodeCritical*100)
	fmt.Fprintln(out)

	if level == supervisor.PressureNone {
		fmt.Fprintln(out, "Status: OK (all metrics within limits)")
		return
	}
	fmt.Fprintf(out, "Status: ALARM (%s)\n", level)
	for _, r := range reasons {
		fmt.Fprintf(out, "  ! %s\n", r)
	}
	switch {
	case cfg.dryRun:
		fmt.Fprintln(out, "Remediation: skipped (--dry-run)")
	case rem == nil:
		fmt.Fprintln(out, "Remediation: none")
	case rem.Error != "":
		fmt.Fprintf(out, "Remediation: FAILED (%s)\n", rem.Error)
	default:
		if rem.SandboxGC != nil {
			fmt.Fprintf(out, "Remediation: reaped %d sandbox(es); reaped %d provably-merged worktree(s)",
				rem.SandboxGC.Reaped, len(rem.Removed))
		} else {
			fmt.Fprintf(out, "Remediation: reaped %d provably-merged worktree(s)", len(rem.Removed))
		}
		if len(rem.Failed) > 0 {
			fmt.Fprintf(out, " (%d failed)", len(rem.Failed))
		}
		fmt.Fprintln(out)
	}
}
