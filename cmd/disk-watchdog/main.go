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
// Remediation reuses the sanctioned safe hook `agm worktree sweep --execute`,
// which removes only provably-MERGED, clean worktrees (squash-safe ancestor/PR
// oracle; dirty, active, and unpushed work is never touched — see
// agm/internal/ops.Sweep). No new destructive cleanup is invented here.
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
//
// Exit codes: 0 = within limits; 1 = at least one threshold breached (an alarm
// fired); 2 = usage/runtime error.
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
// on a husk-heavy machine.
const (
	probeTimeout = 15 * time.Second
	sweepTimeout = 10 * time.Minute
)

type config struct {
	jsonOutput bool
	dryRun     bool
	path       string
	agmBin     string
	trailPath  string
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

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	probe := supervisor.NewSysResourceProbe()
	probe.DiskPath = cfg.path
	snap, err := probe.Snapshot(ctx)
	cancel()
	if err != nil {
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

	// Logging the alarm to the trail is best-effort: a trail write failure is a
	// warning, never a reason to suppress the breach exit code — and on a truly
	// full disk the append will fail while the alarm must still exit 1.
	if breached {
		if lerr := logAlarm(context.Background(), cfg, snap, level, reasons, remediation); lerr != nil {
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
	Removed []string          `json:"removed"`
	Failed  map[string]string `json:"failed,omitempty"`
	Error   string            `json:"error,omitempty"`
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
			// Capture stdout only; the sweep logs progress to stderr, which we
			// intentionally drop so JSON parsing sees clean output.
			return exec.CommandContext(ctx, name, args...).Output()
		}
	}

	out, err := run(ctx, cfg.agmBin, "worktree", "sweep", "--execute", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("%s worktree sweep --execute: %w", cfg.agmBin, err)
	}
	var r sweepResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse worktree sweep output: %w", err)
	}
	return &r, nil
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
		fmt.Fprintf(out, "Remediation: reaped %d provably-merged worktree(s)", len(rem.Removed))
		if len(rem.Failed) > 0 {
			fmt.Fprintf(out, " (%d failed)", len(rem.Failed))
		}
		fmt.Fprintln(out)
	}
}
