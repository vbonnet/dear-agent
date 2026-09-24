// Package gcloghealth interprets the sandbox-GC JSONL log as evidence of a
// scheduled reaper's liveness. Presentation and remediation stay with callers.
package gcloghealth

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

// CompletedOperation is the modern whole-sweep heartbeat wire value.
const CompletedOperation = "sandbox_gc_completed"

// WatchdogSource marks remediation that cannot prove scheduler liveness.
const WatchdogSource = "disk-watchdog"

const (
	operationPrefix    = "sandbox_gc"
	clockSkewTolerance = 5 * time.Minute
	maxRecordBytes     = 1024 * 1024
)

// DefaultInitialBytes is the first whole-record tail window.
const DefaultInitialBytes int64 = 8 * 1024 * 1024

// DefaultMaxBytes is the hard cap for widening a liveness scan.
const DefaultMaxBytes int64 = 128 * 1024 * 1024

// Entry is the subset of the producer's JSONL wire format needed for health.
// The producer lives below agm/internal and cannot be imported by root cmds.
type Entry struct {
	Timestamp     time.Time `json:"timestamp"`
	Operation     string    `json:"operation"`
	Source        string    `json:"source,omitempty"`
	Error         string    `json:"error,omitempty"`
	DryRun        bool      `json:"dry_run,omitempty"`
	Errors        int       `json:"errors,omitempty"`
	ProbeFailures int       `json:"probe_failures,omitempty"`
}

// Options bounds one observation and identifies records that cannot prove the
// scheduled reaper is alive. Zero byte bounds use the package defaults.
type Options struct {
	Now            time.Time
	MaxAge         time.Duration
	InitialBytes   int64
	MaxBytes       int64
	IgnoredSources []string
}

// Summary reports evidence, not an alarm severity or command exit code.
type Summary struct {
	LastSuccess time.Time
	// HasCompletion records any modern completion, including an excluded
	// watchdog completion. Excluded records cannot prove liveness, but they
	// still disqualify historical untagged reap records as legacy fallback.
	HasCompletion          bool
	LastReap               time.Time
	LastError              string
	LastErrorAt            time.Time
	LastFutureCompletionAt time.Time
	OldestSeen             time.Time
	Truncated              bool
	Indeterminate          bool
}

// Proof returns qualified scheduled-reaper evidence. Reap fallback is safe
// only after a complete scan has found no modern completion record.
func (s Summary) Proof() (time.Time, bool) {
	// A stale completion in a capped tail is an observation, not proof that
	// no fresher completion exists in unseen earlier bytes.
	if s.Indeterminate {
		return time.Time{}, false
	}
	if !s.LastSuccess.IsZero() {
		return s.LastSuccess, false
	}
	if !s.Truncated && !s.Indeterminate && !s.HasCompletion {
		return s.LastReap, !s.LastReap.IsZero()
	}
	return time.Time{}, false
}

// Scan reads bounded whole records, widening while unseen history could
// change the liveness answer. A legacy reap is accepted only if a complete
// scan establishes that the log contains no modern completion record.
func Scan(path string, opts Options) (Summary, error) {
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		return Summary{}, err
	}
	ignored := ignoredSourceSet(opts.IgnoredSources)

	window := opts.InitialBytes
	for {
		s, err := scanWindow(path, opts.Now, window, ignored)
		if err != nil {
			return s, err
		}
		if scanIsComplete(s, opts) {
			return s, nil
		}
		if window >= opts.MaxBytes {
			s.Indeterminate = true
			return s, nil
		}
		window *= 2
		if window > opts.MaxBytes {
			window = opts.MaxBytes
		}
	}
}

func normalizeOptions(opts Options) (Options, error) {
	if opts.Now.IsZero() {
		return opts, errors.New("GC log scan requires an evaluation time")
	}
	if opts.InitialBytes <= 0 {
		opts.InitialBytes = DefaultInitialBytes
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.InitialBytes > opts.MaxBytes {
		opts.InitialBytes = opts.MaxBytes
	}
	return opts, nil
}

func ignoredSourceSet(sources []string) map[string]bool {
	ignored := make(map[string]bool, len(sources))
	for _, source := range sources {
		ignored[source] = true
	}
	return ignored
}

func scanIsComplete(s Summary, opts Options) bool {
	if !s.Truncated {
		return true
	}
	// A recent completion is positive proof. Event timestamps elsewhere in a
	// byte tail cannot prove that unseen earlier bytes contain no fresh proof:
	// a clock rollback or backdated append breaks that assumed ordering.
	return opts.MaxAge > 0 && !s.LastSuccess.IsZero() &&
		opts.Now.Sub(s.LastSuccess) <= opts.MaxAge
}

func scanWindow(path string, now time.Time, window int64, ignored map[string]bool) (Summary, error) {
	var s Summary
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()

	s.Truncated, err = seekTail(f, window)
	if err != nil {
		return s, err
	}
	horizon := now.Add(clockSkewTolerance)
	r := bufio.NewReaderSize(f, maxRecordBytes)
	for {
		line, readErr := r.ReadSlice('\n')
		if readErr != nil {
			switch {
			case errors.Is(readErr, bufio.ErrBufferFull):
				if err := discardOversizedRecord(r); err != nil {
					return s, err
				}
				continue
			case errors.Is(readErr, io.EOF):
				if len(line) > 0 {
					s.observe(line, horizon, ignored)
				}
				return s, nil
			default:
				return s, readErr
			}
		}
		s.observe(line, horizon, ignored)
	}
}

func seekTail(f *os.File, window int64) (bool, error) {
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
	skip := bufio.NewReaderSize(f, maxRecordBytes)
	discarded := 0
	for {
		chunk, err := skip.ReadSlice('\n')
		discarded += len(chunk)
		if err == nil || errors.Is(err, io.EOF) {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return true, err
		}
	}
	_, err = f.Seek(info.Size()-window+int64(discarded), io.SeekStart)
	return true, err
}

func discardOversizedRecord(r *bufio.Reader) error {
	for {
		_, err := r.ReadSlice('\n')
		if err == nil || errors.Is(err, io.EOF) {
			return nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}

func (s *Summary) observe(line []byte, horizon time.Time, ignored map[string]bool) {
	var e Entry
	if json.Unmarshal(line, &e) != nil {
		return
	}
	s.noteOldest(e.Timestamp, horizon)
	// A completion dated beyond the skew horizon is not evidence that GC ran.
	// Setting HasCompletion from one made Proof() suppress an otherwise valid
	// legacy reap, so the watchdog reported that GC never completed, which is
	// the opposite of DW-22's requirement to ignore future records.
	//
	// LastFutureCompletionAt below is deliberately still recorded: whether a
	// log contains a future-dated completion is sweep-health's own DOWN
	// signal, and a separate question from whether GC completed.
	if e.Operation == CompletedOperation && !e.Timestamp.IsZero() &&
		!e.Timestamp.After(horizon) {
		s.HasCompletion = true
	}
	if !admitted(e, ignored) {
		return
	}
	if e.Timestamp.After(horizon) {
		if e.Operation == CompletedOperation && e.Timestamp.After(s.LastFutureCompletionAt) {
			s.LastFutureCompletionAt = e.Timestamp
		}
		return
	}
	s.fold(e)
}

func (s *Summary) noteOldest(timestamp, horizon time.Time) {
	if !timestamp.IsZero() && !timestamp.After(horizon) &&
		(s.OldestSeen.IsZero() || timestamp.Before(s.OldestSeen)) {
		s.OldestSeen = timestamp
	}
}

func admitted(e Entry, ignored map[string]bool) bool {
	return !e.Timestamp.IsZero() && strings.HasPrefix(e.Operation, operationPrefix) && !ignored[e.Source]
}

func (s *Summary) fold(e Entry) {
	switch {
	case healthyCompletion(e):
		s.HasCompletion = true
		if e.Timestamp.After(s.LastSuccess) {
			s.LastSuccess = e.Timestamp
		}
	case e.Operation == CompletedOperation:
		s.HasCompletion = true
		if reason := rejectedCompletionReason(e); reason != "" && e.Timestamp.After(s.LastErrorAt) {
			s.LastErrorAt, s.LastError = e.Timestamp, reason
		}
	case e.Operation == "sandbox_gc_reap" && e.Source == "":
		// Only pre-source-tag reaps are eligible for the legacy fallback.
		// A modern runner can emit a reap then terminate before completion.
		if e.Timestamp.After(s.LastReap) {
			s.LastReap = e.Timestamp
		}
	case e.Error != "" && e.Timestamp.After(s.LastErrorAt):
		s.LastErrorAt, s.LastError = e.Timestamp, e.Error
	}
}

func healthyCompletion(e Entry) bool {
	return e.Operation == CompletedOperation && !e.DryRun && e.Errors == 0 && e.ProbeFailures == 0
}

func rejectedCompletionReason(e Entry) string {
	if e.Operation != CompletedOperation {
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
