// Reaper liveness is a leading disk-health indicator. The shared observer
// decides what the GC log proves; this adapter owns watchdog alarm policy.
package main

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/internal/gcloghealth"
)

type gcHealth struct {
	Stale         bool
	LastSuccess   time.Time
	Age           time.Duration
	LastError     string
	Indeterminate bool
	Reason        string
}

// The remediation producer tag must not certify the scheduled reaper.
const gcSelfSource = brakeSource

// Keep the bounds injectable for the command's existing focused tests.
var maxGCLogScanBytes int64 = gcloghealth.DefaultInitialBytes
var maxGCLogTotalScanBytes int64 = gcloghealth.DefaultMaxBytes

// gcLogEntry remains a wire-format alias for producer-tag adapter tests.
type gcLogEntry = gcloghealth.Entry

func oldestSeenLabel(t time.Time) string {
	if t.IsZero() {
		return "an unknown point in time"
	}
	return t.UTC().Format(time.RFC3339)
}

func scanGCLog(path string, now time.Time, maxAge time.Duration) (gcloghealth.Summary, error) {
	return gcloghealth.Scan(path, gcloghealth.Options{
		Now:            now,
		MaxAge:         maxAge,
		InitialBytes:   maxGCLogScanBytes,
		MaxBytes:       maxGCLogTotalScanBytes,
		IgnoredSources: []string{gcSelfSource},
	})
}

// checkGCHealth maps evidence to the watchdog's stale/WARN policy. A missing
// or unreadable log is stale; this check exists for a reaper that stopped.
func checkGCHealth(cfg config, now time.Time) *gcHealth {
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

	return gcHealthFromSummary(summary, cfg, now)
}

// gcHealthFromSummary is the scan seam, so the verdict can be tested without
// standing up a log large enough to cap a scan.
func gcHealthFromSummary(summary gcloghealth.Summary, cfg config, now time.Time) *gcHealth {
	// Old `agm` builds emitted reap records but no completion heartbeat.
	// This fallback is valid only after a complete scan has ruled out a
	// modern completion anywhere in the log.
	last, viaFallback := summary.Proof()

	h := &gcHealth{LastSuccess: last}
	// Order the error against the success actually OBSERVED, not against
	// Proof(). A capped tail holding an old error and a newer-but-stale
	// completion makes Proof() return zero, and comparing against zero ranked
	// the already-superseded error as the newest thing in the log and
	// appended it to the alarm. LastSuccess is the observation; Proof() is a
	// verdict about whether that observation can be trusted as liveness,
	// which is a different question from which record came last.
	errorBaseline := summary.LastSuccess
	if errorBaseline.IsZero() {
		errorBaseline = last
	}
	if summary.LastErrorAt.After(errorBaseline) {
		h.LastError = summary.LastError
	}
	switch {
	case last.IsZero() && summary.Indeterminate:
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
