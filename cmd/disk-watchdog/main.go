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
//	disk-watchdog --gc-max-age 2h        # reaper-staleness window (0 disables)
//	disk-watchdog --gc-log /path/gc.jsonl   # sandbox-GC log to read liveness from
//
// # Reaper liveness
//
// Free space is a lagging indicator of a leaked-sandbox problem: by the time it
// crosses the 20 GiB floor, hundreds of GB have already accumulated. So the
// watchdog also alarms when the hourly sandbox GC has stopped completing sweeps
// (--gc-max-age, default 6h), independently of how much space is free.
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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

	// Read reaper liveness BEFORE remediating. Remediation runs the sandbox
	// sweep itself, which appends a completion record; evaluating afterwards
	// would grade the schedule on a heartbeat this tick just produced. The
	// producer tag (gcSelfSource) keeps later ticks from counting it too, and
	// this ordering keeps the current tick honest even against an older `agm`
	// that does not stamp the tag.
	gc := checkGCHealth(cfg, time.Now())

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
		lerr := logAlarm(logCtx, cfg, snap, level, reasons, remediation, gc)
		logCancel()
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "disk-watchdog: warning: trail append failed: %v\n", lerr)
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(out, snap, level, reasons, remediation, cfg, gc); err != nil {
			return 2, err
		}
	} else {
		emitReport(out, snap, level, reasons, remediation, cfg, gc)
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

// gcHealth is what a tick concluded about the sandbox reaper itself.
//
// Disk-free is a *lagging* indicator of a leaked-sandbox problem: by the time
// free space crosses the 20 GiB WARN floor, hundreds of GB of sandboxes have
// already accumulated and the cheap remediations are gone. A reaper that has
// stopped completing sweeps is the *leading* indicator, and it is independent
// of how much space happens to be free right now.
//
// This is the generalisation of the ce-93lw.18 lesson. That fix made the
// watchdog's own failed remediation consume-able (it latches the brake). The
// same reasoning was never applied to the hourly sandbox GC, so when that job
// began exiting non-zero on 2026-07-05 it wrote one line an hour to a log with
// no reader, for a month, while ~/.agm/sandboxes grew to 239 GB.
type gcHealth struct {
	// Stale is true when no successful sweep is recent enough (or none exists).
	Stale bool
	// LastSuccess is the newest completed sweep; zero when none was found.
	LastSuccess time.Time
	// Age of LastSuccess at tick time; only meaningful when LastSuccess is set.
	Age time.Duration
	// LastError is the newest GC error message, when the log records one after
	// the last success. It turns "the reaper is stale" into an actionable line.
	LastError string
	// Indeterminate is true when liveness could not be established either way:
	// no heartbeat was found, but the scan did not read far enough back to
	// prove none exists. It is reported as an alarm — an unanswerable question
	// is not a healthy answer — but distinctly from a reaper that provably
	// never ran, because the two have different suspects and different fixes.
	Indeterminate bool
	// Reason is the human-readable summary for the alarm and the report.
	Reason string
}

// oldestSeenLabel renders the reach of a truncated scan for an operator.
func oldestSeenLabel(t time.Time) string {
	if t.IsZero() {
		return "an unknown point in time"
	}
	return t.UTC().Format(time.RFC3339)
}

// gcLogEntry is the subset of agm/internal/gclog.Entry this watchdog reads.
type gcLogEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	Operation     string    `json:"operation"`
	Source        string    `json:"source,omitempty"`
	Error         string    `json:"error,omitempty"`
	DryRun        bool      `json:"dry_run,omitempty"`
	Errors        int       `json:"errors,omitempty"`
	ProbeFailures int       `json:"probe_failures,omitempty"`
}

// The operation emitted once per successful sweep, reap or no reap. A sweep
// that reaps nothing is still a healthy sweep, so counting reap records alone
// would call a correctly-idle reaper dead.
const gcCompletedOperation = "sandbox_gc_completed"

// gcOperationPrefix scopes this reader to sandbox-GC records. gc.jsonl is
// shared with the session GC, which writes its own failures (`gc_archive_error`
// and friends). Attributing one of those to a stale sandbox reaper would point
// whoever reads the alarm at an unrelated subsystem.
const gcOperationPrefix = "sandbox_gc"

// gcSelfSource is the value this watchdog stamps on the sweeps it triggers
// itself (remediationEnv), and the one value the liveness check must ignore.
//
// Remediation runs `agm sandbox gc --reap` on every breached tick, which writes
// a fresh completion record. Counting those, the question "is the hourly
// schedule still alive?" is answered by evidence this process manufactured five
// minutes ago: under sustained disk pressure a dead schedule would look healthy
// indefinitely, exactly inverting the leading indicator. An unstamped record is
// NOT assumed to be ours — a manual run or an older agm leaves the field empty,
// and treating unknown as self would discard real heartbeats.
const gcSelfSource = brakeSource

// gcClockSkewTolerance bounds how far ahead of now a heartbeat may be dated and
// still count. An append-only log keeps the maximum timestamp forever, so a
// single heartbeat written while the host clock ran fast would otherwise yield
// a negative age — permanently "healthy" — and no later correctly-dated record
// could displace it.
const gcClockSkewTolerance = 5 * time.Minute
const maxGCLogRecordBytes = 1024 * 1024

// maxGCLogScanBytes bounds how much of gc.jsonl one tick reads.
//
// gc.jsonl is append-only and shared with the session GC, so on a long-lived
// host it grows without bound. Reading from byte zero every five minutes makes
// the work per tick grow with total history, and launchd does not overlap
// instances of the same job — a slow enough scan silently starves every
// later disk sample and alarm. Only the newest heartbeat and the newest error
// matter, so read a fixed tail and start at the first whole record inside it.
// A var, not a const, so a test can exercise the widening below without
// writing hundreds of megabytes to a disk this tool exists to protect.
var maxGCLogScanBytes int64 = 8 * 1024 * 1024

// maxGCLogTotalScanBytes caps the escalation below.
//
// A fixed tail alone answers the wrong question. Record volume is not bounded
// by elapsed time: enough session-GC chatter after a healthy sandbox sweep
// pushes that heartbeat out of the window, and the scan then reports "no
// completed sweep was ever recorded" for a reaper that is well inside its SLA.
// So when the first window holds no heartbeat, widen it — but only while older
// history could still change the answer, i.e. while the window has not yet
// reached back past the liveness horizon. Beyond this hard cap the scan stops
// and reports that it could not determine liveness, which is a different
// answer from "the reaper never ran" and is reported as such.
var maxGCLogTotalScanBytes int64 = 128 * 1024 * 1024

// healthyHeartbeat reports whether a completion record is evidence the reaper
// actually did its job. A dry run reclaims nothing, a sweep whose deletion
// attempts failed leaves the sandboxes in place, and a sweep whose safety
// gates couldn't even run (lsof/mount table/session store unreadable) proves
// nothing was evaluated — every entry it "kept" was a refusal to look, not a
// finding. Counting any of the three would let a broken reaper suppress its
// own alarm indefinitely.
func (e gcLogEntry) healthyHeartbeat() bool {
	return e.Operation == gcCompletedOperation && !e.DryRun && e.Errors == 0 && e.ProbeFailures == 0
}

// rejectedCompletionReason explains why a completion record was not accepted
// as a liveness heartbeat, or "" when it was healthy.
func rejectedCompletionReason(e gcLogEntry) string {
	if e.Operation != gcCompletedOperation {
		return ""
	}
	var causes []string
	if e.DryRun {
		causes = append(causes, "dry run reclaimed nothing")
	}
	if e.Errors > 0 {
		causes = append(causes, fmt.Sprintf("%d deletion error(s)", e.Errors))
	}
	if e.ProbeFailures > 0 {
		causes = append(causes, fmt.Sprintf("%d safety-probe failure(s)", e.ProbeFailures))
	}
	if len(causes) == 0 {
		return ""
	}
	return "sweep completed without a healthy heartbeat: " + strings.Join(causes, ", ")
}

// gcLogSummary is what one pass over the GC log yields.
type gcLogSummary struct {
	// LastSuccess is the newest heartbeat that proves a real, error-free sweep.
	LastSuccess time.Time
	// HasCompletion is true when the log contains a modern completion record.
	// Once the producer is modern, reap records are no longer an acceptable
	// liveness fallback because a partial-failure run can emit both.
	HasCompletion bool
	// LastReap is the newest individual reap record. A pre-heartbeat `agm` emits
	// these but no completion record, so they are the compatibility signal that
	// keeps a newer watchdog from calling an older-but-working GC dead.
	LastReap    time.Time
	LastError   string
	LastErrorAt time.Time
	// OldestSeen is the oldest timestamped record inside the scanned window,
	// of any GC kind. With Truncated it bounds what the unscanned history could
	// still contain: every record before the window is older than this.
	OldestSeen time.Time
	// Truncated is true when the scan started past byte zero, so records older
	// than OldestSeen exist but were not read.
	Truncated bool
	// Indeterminate is true when the scan found no heartbeat, stopped at the
	// hard byte cap, and the unread history is recent enough that it could
	// still hold one inside the liveness window. The caller must not report
	// this as "the reaper never completed a sweep": absent and could-not-tell
	// are different answers, and only one of them names the right suspect.
	Indeterminate bool
}

// scanGCLog reads every sandbox-GC record in the log and keeps the newest
// healthy heartbeat, the newest reap, and the newest error.
//
// It reads with a bufio.Reader rather than a Scanner deliberately. A Scanner
// aborts the whole scan with ErrTooLong on any line above its buffer cap, which
// on an append-only log means one oversized or corrupt record would hide every
// valid heartbeat appended after it. Oversized records are discarded with a
// fixed-size bound, then scanning resumes at the next record. Genuine I/O
// errors are returned rather than swallowed.
// The window widens only while a wider one could still change the answer:
// once a heartbeat is found, once the window reaches byte zero, or once its
// oldest record predates the liveness window (nothing before it can be a
// non-stale heartbeat), reading further is wasted I/O on a disk already under
// pressure. maxAge <= 0 disables the widening entirely.
func scanGCLog(path string, now time.Time, maxAge time.Duration) (gcLogSummary, error) {
	window := maxGCLogScanBytes
	for {
		s, err := scanGCLogWindow(path, now, window)
		switch {
		case err != nil:
			return s, err
		case !s.Truncated || !s.LastSuccess.IsZero() || !s.LastReap.IsZero():
			return s, nil
		case maxAge <= 0:
			return s, nil
		case !s.OldestSeen.IsZero() && now.Sub(s.OldestSeen) > maxAge:
			// Everything still unread is older than a record that is already
			// outside the SLA, so no heartbeat there could be a live one.
			// "Stale" is a sound conclusion; stop.
			return s, nil
		case window >= maxGCLogTotalScanBytes:
			s.Indeterminate = true
			return s, nil
		}
		if window *= 2; window > maxGCLogTotalScanBytes {
			window = maxGCLogTotalScanBytes
		}
	}
}

// scanGCLogWindow folds the last `window` bytes of the log into a summary.
func scanGCLogWindow(path string, now time.Time, window int64) (gcLogSummary, error) {
	var s gcLogSummary
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()

	horizon := now.Add(gcClockSkewTolerance)
	truncated, err := seekGCLogTail(f, window)
	if err != nil {
		return s, err
	}
	s.Truncated = truncated
	r := bufio.NewReaderSize(f, maxGCLogRecordBytes)
	for {
		line, readErr := r.ReadSlice('\n')
		if readErr != nil {
			if errors.Is(readErr, bufio.ErrBufferFull) {
				if err := discardOversizedGCLogRecord(r); err != nil {
					return s, err
				}
				continue
			}
			if errors.Is(readErr, io.EOF) {
				if len(line) > 0 {
					s.observe(line, horizon)
				}
				return s, nil
			}
			return s, readErr
		}
		s.observe(line, horizon)
	}
}

// seekGCLogTail positions f at the start of the first whole record within the
// last `window` bytes, and reports whether anything before that point exists.
// A file at or below the bound is read in full. The partial record straddling
// the cut is skipped rather than parsed, so a truncated line is never mistaken
// for a malformed one.
func seekGCLogTail(f *os.File, window int64) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() <= window {
		return false, nil
	}
	if _, err := f.Seek(info.Size()-window, io.SeekStart); err != nil {
		return true, err
	}
	// Drop bytes up to and including the next newline: they are the tail of a
	// record whose head lies before the cut.
	skip := bufio.NewReaderSize(f, maxGCLogRecordBytes)
	discarded := 0
	for {
		chunk, err := skip.ReadSlice('\n')
		discarded += len(chunk)
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return true, err
	}
	_, err = f.Seek(info.Size()-window+int64(discarded), io.SeekStart)
	return true, err
}

func discardOversizedGCLogRecord(r *bufio.Reader) error {
	for {
		_, err := r.ReadSlice('\n')
		if err == nil {
			return nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

// observe folds one raw log line into the summary. Records dated beyond the
// clock-skew horizon are discarded outright.
func (s *gcLogSummary) observe(line []byte, horizon time.Time) {
	e, ok := s.admit(line, horizon)
	if !ok {
		return
	}
	s.fold(e)
}

// admit decodes one line and reports whether it is liveness evidence at all,
// recording the window's reach on the way through.
func (s *gcLogSummary) admit(line []byte, horizon time.Time) (gcLogEntry, bool) {
	var e gcLogEntry
	if json.Unmarshal(line, &e) != nil {
		return e, false
	}
	if e.Timestamp.After(horizon) {
		return e, false
	}
	// Track how far back this window reaches before filtering by operation: the
	// bound is a property of the file, and the log is shared with the session
	// GC, so the oldest record of ANY kind is what says whether history older
	// than the window could still hold a heartbeat inside the liveness SLA.
	if !e.Timestamp.IsZero() && (s.OldestSeen.IsZero() || e.Timestamp.Before(s.OldestSeen)) {
		s.OldestSeen = e.Timestamp
	}
	if !strings.HasPrefix(e.Operation, gcOperationPrefix) {
		return e, false
	}
	// A sweep this watchdog triggered says nothing about the schedule it is
	// watching, in either direction: its successes are self-manufactured proof
	// of life, and its failures are already surfaced as this tick's remediation
	// outcome. Drop it from the liveness evidence entirely.
	if e.Source == gcSelfSource {
		return e, false
	}
	return e, true
}

// fold merges one admitted sandbox-GC record into the summary.
func (s *gcLogSummary) fold(e gcLogEntry) {
	switch {
	case e.healthyHeartbeat():
		s.HasCompletion = true
		if e.Timestamp.After(s.LastSuccess) {
			s.LastSuccess = e.Timestamp
		}
	case e.Operation == gcCompletedOperation:
		// A completion that is not a healthy heartbeat proves the reaper ran
		// and did not do its job. The producer records that as structured
		// counts with no `error` field, so without promoting them here the
		// alarm would report only "no proof of life" once the last healthy
		// heartbeat went stale — leaving a responder unable to tell a dead
		// schedule from a schedule that runs and fails every time (DW-19).
		s.HasCompletion = true
		if reason := rejectedCompletionReason(e); reason != "" && e.Timestamp.After(s.LastErrorAt) {
			s.LastErrorAt, s.LastError = e.Timestamp, reason
		}
	case e.Operation == "sandbox_gc_reap":
		if e.Timestamp.After(s.LastReap) {
			s.LastReap = e.Timestamp
		}
	case e.Error != "" && e.Timestamp.After(s.LastErrorAt):
		s.LastErrorAt, s.LastError = e.Timestamp, e.Error
	}
}

// checkGCHealth classifies reaper liveness from the GC log. A missing or
// unreadable log is treated as stale: the check exists precisely for the case
// where the reaper is not running, and that case often means no log at all.
func checkGCHealth(cfg config, now time.Time) *gcHealth {
	// Zero, and only zero, disables the check: `run` rejects a negative window
	// as a usage error rather than reading a typo as "monitoring off".
	if cfg.gcLogPath == "" || cfg.gcMaxAge == 0 {
		return nil
	}
	summary, err := scanGCLog(cfg.gcLogPath, now, cfg.gcMaxAge)
	if err != nil {
		return &gcHealth{
			Stale: true,
			Reason: fmt.Sprintf("sandbox GC log %s is unreadable (%v); reaper liveness cannot be confirmed",
				cfg.gcLogPath, err),
		}
	}

	// Compatibility with an `agm` predating the heartbeat: such a build emits
	// per-sandbox reap records but no completion record, so a recent reap is
	// accepted as proof of life. Errors are deliberately NOT accepted here —
	// treating a failing GC's own error records as evidence it is alive would
	// mask exactly the failure this check exists to catch.
	last, viaFallback := summary.LastSuccess, false
	if !summary.HasCompletion && summary.LastReap.After(last) {
		last, viaFallback = summary.LastReap, true
	}

	h := &gcHealth{LastSuccess: last}
	// Only surface an error newer than the last proof of life; an older error
	// that a later successful sweep superseded is noise.
	if summary.LastErrorAt.After(last) {
		h.LastError = summary.LastError
	}
	switch {
	case last.IsZero() && summary.Indeterminate:
		// The scan hit its hard byte cap without reaching back past the
		// liveness window, so a heartbeat inside the SLA may still sit in the
		// unread history. Alarm — a watchdog that cannot answer its own
		// question is not reporting health — but never claim the reaper never
		// ran: that names the wrong suspect and sends a responder to restart a
		// job whose real problem is the volume of this log.
		h.Stale = true
		h.Indeterminate = true
		h.Reason = fmt.Sprintf(
			"sandbox GC liveness is undetermined: the newest %d MiB of %s holds no completed sweep "+
				"and older history was not scanned (records back to %s)",
			maxGCLogTotalScanBytes/(1024*1024), cfg.gcLogPath, oldestSeenLabel(summary.OldestSeen))
	case last.IsZero():
		h.Stale = true
		h.Reason = fmt.Sprintf("sandbox GC has never recorded a completed sweep in %s", cfg.gcLogPath)
	default:
		h.Age = now.Sub(last)
		if h.Age > cfg.gcMaxAge {
			h.Stale = true
			h.Reason = fmt.Sprintf("sandbox GC last completed a sweep %s ago (max %s)",
				h.Age.Round(time.Minute), cfg.gcMaxAge)
		}
	}
	if h.Stale && h.LastError != "" {
		h.Reason += fmt.Sprintf("; last GC error: %s", h.LastError)
	}
	if h.Stale && viaFallback {
		h.Reason += "; liveness inferred from reap records — installed agm predates the sandbox_gc_completed heartbeat"
	}
	return h
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
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, gc *gcHealth) error {
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

	return trail.Append(ctx, decisiontrail.Record{
		Role:    "watchdog",
		Kind:    "watchdog.disk.alarm",
		Payload: payload,
	})
}

func emitJSON(out io.Writer, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, cfg config, gc *gcHealth) error {
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
		OK:                level == supervisor.PressureNone,
	})
}

func emitReport(out io.Writer, snap supervisor.ResourceSnapshot,
	level supervisor.PressureLevel, reasons []string, rem *sweepResult, cfg config, gc *gcHealth) {
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
