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
// then `agm worktree sweep --execute`. SGC-18 currently makes the sandbox-GC
// request fail closed until session-store endpoint transport is authenticated;
// this watchdog reports that as a remediation failure and still evaluates the
// independent worktree sweep. The worktree sweep removes only provably-MERGED,
// clean worktrees. No new destructive cleanup is invented here.
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
//	disk-watchdog --gc-max-age 2h        # reaper-staleness window (0 disables)
//	disk-watchdog --gc-log /path/gc.jsonl   # sandbox-GC log to read liveness from
//
// # Reaper liveness
//
// Free space is a lagging indicator of a leaked-sandbox problem: by the time it
// crosses the 20 GiB floor, hundreds of GB have already accumulated. So the
// watchdog also alarms when the hourly sandbox GC has stopped completing sweeps
// (--gc-max-age, default 6h), independently of how much space is free. During
// SGC-18 containment no destructive completion is possible, so this alarm is an
// intentional report of the unavailable reclamation path.
//
// This generalises ce-93lw.18. That fix made *this* watchdog's failed
// remediation consume-able by latching the admission brake. The same reasoning
// was never applied to the sandbox GC, so when that job started exiting
// non-zero on 2026-07-05 it wrote one line an hour to a log with no reader —
// for a month — while ~/.agm/sandboxes grew to 239 GB across 119 dirs and every
// tick here still printed "Status: OK". A stale reaper alarms and exits 1 but
// deliberately does not latch the brake: halting every spawn because a GC is
// behind would be a worse outage than the leak it warns about.
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
	"strconv"
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
	gcLogPath  string
	gcMaxAge   time.Duration
	thresholds supervisor.DiskAlertThresholds

	// Build-cache reaping is deliberately NOT gated on disk pressure. A Go
	// build cache older than buildCacheMinAge has no value — the next run
	// makes its own — and on this host they accrued ~9 GB/day. Waiting for a
	// breach would mean absorbing that between breaches, which is
	// firefighting rather than a bound.
	buildCacheRoots  string
	buildCacheMinAge time.Duration
	buildCacheDepth  int

	// E2E test fixture directories under the user cache directory also accrue
	// multi-GB build artifacts without bounds (ce-4cp3a).
	e2eCacheDir        string
	e2eCacheMinAge     time.Duration
	e2eCacheMaxEntries int

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
	fs.StringVar(&cfg.gcLogPath, "gc-log", defaultGCLogPath(),
		"sandbox-GC JSONL log consulted for reaper liveness; empty disables the check")
	fs.DurationVar(&cfg.gcMaxAge, "gc-max-age", defaultGCMaxAge,
		"alarm when the sandbox GC has not completed a sweep within this window (0 disables)")
	fs.StringVar(&cfg.buildCacheRoots, "build-cache-roots", defaultBuildCacheRoots(),
		"comma-separated directories scanned for abandoned Go build caches (empty disables the reaper)")
	fs.DurationVar(&cfg.buildCacheMinAge, "build-cache-min-age", defaultBuildCacheMinAge,
		"only reap Go build caches whose mtime is older than this")
	fs.IntVar(&cfg.buildCacheDepth, "build-cache-depth", defaultBuildCacheDepth,
		"how many directory levels below each scan root to search for build caches")
	fs.StringVar(&cfg.e2eCacheDir, "e2e-cache-dir", defaultE2ECacheDir(),
		"directory scanned for abandoned E2E test fixture directories (empty disables the reaper)")
	fs.DurationVar(&cfg.e2eCacheMinAge, "e2e-cache-min-age", defaultE2ECacheMinAge,
		"only reap E2E fixture directories whose mtime is older than this")
	fs.IntVar(&cfg.e2eCacheMaxEntries, "e2e-cache-max-entries", defaultE2ECacheMaxEntries,
		"maximum number of recent E2E fixture directories to retain")
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
	// Only zero disables the liveness check (DW-20). A negative duration parses
	// happily, and silently disabling on it would let one typo in a launchd
	// plist leave a dead reaper unmonitored while every tick still reports OK —
	// the monitoring gap this watchdog exists to close. Refuse it out loud.
	if cfg.gcMaxAge < 0 {
		return 2, fmt.Errorf("invalid -gc-max-age %s: the reaper-liveness window cannot be negative (use 0 to disable the check)", cfg.gcMaxAge)
	}
	// A negative or zero age window would make every cache — including one a
	// build is writing right now — instantly eligible. Refuse it rather than
	// silently reaping live state.
	if cfg.buildCacheRoots != "" && cfg.buildCacheMinAge <= 0 {
		return 2, fmt.Errorf("invalid -build-cache-min-age %s: must be positive (pass an empty -build-cache-roots to disable the reaper)", cfg.buildCacheMinAge)
	}
	if cfg.e2eCacheDir != "" && cfg.e2eCacheMinAge <= 0 {
		return 2, fmt.Errorf("invalid -e2e-cache-min-age %s: must be positive (pass an empty -e2e-cache-dir to disable the reaper)", cfg.e2eCacheMinAge)
	}
	if cfg.e2eCacheDir != "" && cfg.e2eCacheMaxEntries < 0 {
		return 2, fmt.Errorf("invalid -e2e-cache-max-entries %d: must be non-negative", cfg.e2eCacheMaxEntries)
	}

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
	// Disk pressure alone drives remediation and the admission brake. The
	// worktree sweep shells out to gh per worktree and costs minutes, so it must
	// stay tied to actual space pressure — a stale reaper on a host with 200 GiB
	// free is a bug to report, not a reason to re-sweep every five minutes.
	diskBreached := level != supervisor.PressureNone

	// Read reaper liveness BEFORE remediating. SGC-18 currently refuses the
	// sandbox request before it can append a completion record. Once authenticated
	// transport restores destructive execution, remediation can append one;
	// evaluating afterwards would grade the schedule on a heartbeat this tick
	// just produced. The producer tag (gcSelfSource) keeps later ticks from
	// counting it too, and this ordering keeps the current tick honest even
	// against an older `agm` that does not stamp the tag.
	gc := checkGCHealth(cfg, time.Now())

	// Runs on every tick, breached or not: see the comment on config.
	buildCaches := reapAbandonedBuildCaches(cfg)
	e2eCaches := reapAbandonedE2ECaches(cfg)

	var remediation *sweepResult
	if diskBreached && !cfg.dryRun {
		r, rerr := sweepMergedWorktrees(context.Background(), cfg)
		if rerr != nil {
			// Remediation failure must not hide the alarm; record it and keep going.
			r = &sweepResult{Error: rerr.Error()}
		}
		remediation = r
	}

	updateAdmissionBrake(cfg, diskBreached, remediation)

	// A dead reaper is an alarm in its own right, at whatever free space happens
	// to be. Folding it into the same level/reasons the disk thresholds produce
	// routes it through the existing trail and exit-code paths instead of adding
	// a parallel notification channel to keep in sync. It deliberately does not
	// touch the brake: blocking every spawn because a GC is behind would be a
	// worse outage than the leak it warns about.
	if gc != nil && gc.Stale {
		if level == supervisor.PressureNone {
			level = supervisor.PressureWarn
		}
		reasons = append(reasons, gc.Reason)
	}
	breached := level != supervisor.PressureNone

	// Logging the alarm to the trail is best-effort: a trail write failure is a
	// warning, never a reason to suppress the breach exit code — and on a truly
	// full disk the append will fail while the alarm must still exit 1. The
	// write is timeout-bounded because I/O on an exhausted disk can stall.
	if breached {
		logCtx, logCancel := context.WithTimeout(context.Background(), trailTimeout)
		lerr := logAlarm(logCtx, cfg, snap, level, reasons, remediation, gc, buildCaches, e2eCaches)
		logCancel()
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "disk-watchdog: warning: trail append failed: %v\n", lerr)
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(out, snap, level, reasons, remediation, cfg, gc, buildCaches, e2eCaches); err != nil {
			return 2, err
		}
	} else {
		emitReport(out, snap, level, reasons, remediation, cfg, gc, buildCaches, e2eCaches)
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

// defaultGCMaxAge tolerates several missed hourly sweeps before alarming, so a
// transient Dolt restart does not page, but a genuinely dead reaper surfaces
// the same day rather than a month later.
const defaultGCMaxAge = 6 * time.Hour

func defaultGCLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/logs/gc.jsonl"
	}
	return filepath.Join(home, ".agm", "logs", "gc.jsonl")
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
	// DryRun and ReapRefused are how a refused reap reaches this consumer.
	// This watchdog always passes --reap, so `dry_run: true` in the reply means
	// the sweep declined to delete anything (today: a partial live-session
	// inventory). In that state `Reaped` counts would-reaps, not deletions —
	// reading it as reclaimed space would report remediation that never
	// happened and leave the admission brake open on a host that is still
	// filling up. ReapRefused carries the reason from a sweep new enough to
	// state it; DryRun is the signal from one that is not.
	DryRun      bool   `json:"dry_run,omitempty"`
	ReapRefused string `json:"reap_refused,omitempty"`
}

// refusalError renders a refused reap as a remediation failure, or "" when the
// sweep did what it was asked. A refusal is not an ordinary outcome here: the
// watchdog only invokes the sweep under disk pressure, so a tick that deletes
// nothing has not remediated, and saying so is what latches the brake.
func (s *sandboxGCSummary) refusalError() string {
	if s == nil || s.Error != "" {
		return ""
	}
	switch {
	case s.ReapRefused != "":
		return "requested reap was refused: " + s.ReapRefused
	case s.DryRun:
		return "requested reap was downgraded to a scan; no sandbox was deleted " +
			"(reaped=" + strconv.Itoa(s.Reaped) + " counts would-reaps)"
	default:
		return ""
	}
}

// remediationEnv is the environment overlay for every command this watchdog
// shells out to.
//
// GIT_TERMINAL_PROMPT: the sweep shells out to git/gh; with no TTY a credential
// prompt would hang the launchd job until the timeout.
//
// AGM_GC_SOURCE: the sandbox sweep stamps this on every record it writes, so
// the liveness check below can tell the hourly schedule's heartbeats from the
// ones this watchdog's own remediation just produced. Without it, remediation
// forges proof of life for the very schedule it is meant to be watching.
func remediationEnv() []string {
	return []string{"GIT_TERMINAL_PROMPT=0", "AGM_GC_SOURCE=" + gcSelfSource}
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
			cmd.Env = append(os.Environ(), remediationEnv()...)
			// Capture stdout only; the sweep logs progress to stderr, which we
			// intentionally drop so JSON parsing sees clean output.
			return cmd.Output()
		}
	}

	result := &sweepResult{}
	gcOut, gcErr := run(ctx, cfg.agmBin, "sandbox", "gc", "--reap", "--json")
	switch {
	case gcErr != nil:
		result.SandboxGC = &sandboxGCSummary{Error: gcErr.Error()}
	default:
		if err := json.Unmarshal(gcOut, &result.SandboxGC); err != nil {
			result.SandboxGC = &sandboxGCSummary{Error: fmt.Sprintf("parse sandbox gc output: %v", err)}
		} else if refusal := result.SandboxGC.refusalError(); refusal != "" {
			result.SandboxGC.Error = refusal
		}
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
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, gc *gcHealth,
	buildCaches *buildCacheReapResult, e2eCaches *e2eCacheReapResult) error {
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
	if gc != nil {
		sandboxGC := map[string]any{"stale": gc.Stale}
		if !gc.LastSuccess.IsZero() {
			sandboxGC["last_success"] = gc.LastSuccess.UTC().Format(time.RFC3339)
			sandboxGC["age_seconds"] = int64(gc.Age.Seconds())
		}
		if gc.LastError != "" {
			sandboxGC["last_error"] = gc.LastError
		}
		payload["sandbox_gc"] = sandboxGC
	}
	if buildCaches != nil {
		payload["build_caches"] = buildCaches
	}
	if e2eCaches != nil {
		payload["e2e_caches"] = e2eCaches
	}

	return trail.Append(ctx, decisiontrail.Record{
		Role:    "watchdog",
		Kind:    "watchdog.disk.alarm",
		Payload: payload,
	})
}

func emitJSON(out io.Writer, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, cfg config, gc *gcHealth,
	buildCaches *buildCacheReapResult, e2eCaches *e2eCacheReapResult) error {
	type gcReport struct {
		Stale       bool   `json:"stale"`
		LastSuccess string `json:"last_success,omitempty"`
		AgeSeconds  int64  `json:"age_seconds,omitempty"`
		LastError   string `json:"last_error,omitempty"`
		// Undetermined distinguishes "the scan could not tell" from "no sweep
		// was ever recorded" for a machine reader, the same way Reason does
		// for a human one.
		Undetermined bool   `json:"undetermined,omitempty"`
		Reason       string `json:"reason,omitempty"`
	}
	type report struct {
		Level             string                         `json:"level"`
		DiskFreeBytes     uint64                         `json:"disk_free_bytes"`
		DiskFreeGiB       float64                        `json:"disk_free_gib"`
		DiskUsedFraction  float64                        `json:"disk_used_fraction"`
		InodeUsedFraction float64                        `json:"inode_used_fraction"`
		Thresholds        supervisor.DiskAlertThresholds `json:"thresholds"`
		Reasons           []string                       `json:"reasons"`
		Remediation       *sweepResult                   `json:"remediation,omitempty"`
		SandboxGC         *gcReport                      `json:"sandbox_gc,omitempty"`
		BuildCaches       *buildCacheReapResult          `json:"build_caches,omitempty"`
		E2ECaches         *e2eCacheReapResult            `json:"e2e_caches,omitempty"`
		OK                bool                           `json:"ok"`
	}
	var gcr *gcReport
	if gc != nil {
		gcr = &gcReport{Stale: gc.Stale, LastError: gc.LastError, Undetermined: gc.Indeterminate, Reason: gc.Reason}
		if !gc.LastSuccess.IsZero() {
			gcr.LastSuccess = gc.LastSuccess.UTC().Format(time.RFC3339)
			gcr.AgeSeconds = int64(gc.Age.Seconds())
		}
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
		SandboxGC:         gcr,
		BuildCaches:       buildCaches,
		E2ECaches:         e2eCaches,
		OK:                level == supervisor.PressureNone,
	})
}

func emitReport(out io.Writer, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, cfg config, gc *gcHealth,
	buildCaches *buildCacheReapResult, e2eCaches *e2eCacheReapResult) {
	fmt.Fprintln(out, "disk-watchdog report")
	fmt.Fprintf(out, "  path        : %s\n", cfg.path)
	fmt.Fprintf(out, "  disk free   : %.1f GiB  [warn < %.0f GiB, critical < %.0f GiB]\n",
		float64(snap.DiskFreeBytes)/supervisor.GiB,
		float64(cfg.thresholds.FreeWarnBytes)/supervisor.GiB,
		float64(cfg.thresholds.FreeCriticalBytes)/supervisor.GiB)
	fmt.Fprintf(out, "  disk used   : %.1f%%\n", snap.DiskUsedFraction*100)
	fmt.Fprintf(out, "  inode usage : %.1f%%  [warn > %.0f%%, critical > %.0f%%]\n",
		snap.InodeUsedFraction*100, cfg.thresholds.InodeWarn*100, cfg.thresholds.InodeCritical*100)
	if gc != nil {
		switch {
		case gc.Indeterminate:
			fmt.Fprintf(out, "  sandbox GC  : UNDETERMINED (log too large to scan back)  [max age %s]\n", cfg.gcMaxAge)
		case gc.LastSuccess.IsZero():
			fmt.Fprintf(out, "  sandbox GC  : NEVER completed a sweep  [max age %s]\n", cfg.gcMaxAge)
		default:
			state := "ok"
			if gc.Stale {
				state = "STALE"
			}
			fmt.Fprintf(out, "  sandbox GC  : %s, last sweep %s ago  [max age %s]\n",
				state, gc.Age.Round(time.Minute), cfg.gcMaxAge)
		}
	}
	if buildCaches != nil {
		fmt.Fprintf(out, "  %s\n", summarizeBuildCacheReap(*buildCaches))
	}
	if e2eCaches != nil {
		fmt.Fprintf(out, "  %s\n", summarizeE2ECacheReap(*e2eCaches))
	}
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
