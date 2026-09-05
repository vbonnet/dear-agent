// Reaper liveness: reading the sandbox-GC log to decide whether the hourly
// reaper is still completing sweeps.
//
// Disk-free is a LAGGING indicator of a leaked-sandbox problem; a reaper that
// has stopped completing sweeps is the LEADING one, and it is independent of
// how much space happens to be free right now. This file owns everything
// between the on-disk gc.jsonl records and the gcHealth verdict a tick folds
// into its alarm: what counts as proof of life, whose records count, how much
// of the log is read, and what to say when the answer cannot be determined.
//
// It lives beside main.go rather than inside it because the reader is a
// cohesive unit with its own failure modes (DW-17 through DW-31), and because
// a watchdog file that grows without bound is the same structural problem this
// tool exists to catch.

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

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
// Remediation requests `agm sandbox gc --reap` on every breached tick. SGC-18
// currently rejects that request before it can write a completion record; when
// authenticated transport restores destructive execution, a successful request
// can again write one. Counting such self-produced records would answer "is the
// hourly schedule still alive?" with evidence this process manufactured five
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
	if !summary.HasCompletion && summary.LastReap.After(last) && summary.LastReap.After(summary.LastErrorAt) {
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
