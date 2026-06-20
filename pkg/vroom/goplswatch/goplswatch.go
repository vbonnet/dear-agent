// Package goplswatch implements the gopls watchdog: it samples gopls process
// pressure (count, resident memory, and system-wide FD-table usage), decides
// whether any threshold is breached, and reports the breaches so a caller can
// log an alarm and remediate.
//
// Background (epic ce-710r, the 2026-06-15 P0): gopls v0.22+ holds ~4800 file
// descriptors per process. When a Claude Code session dies abruptly its gopls
// is reparented to PID 1 and lingers, each instance holding hundreds of MB of
// RSS and thousands of FDs. A handful of orphans is enough to exhaust the
// system file table (kern.maxfiles) and make every `go build` fail with ENFILE.
//
// This package keys alarms on three independent signals:
//
//   - count   — total gopls processes exceeds a ceiling (leak indicator)
//   - rss     — summed resident memory of all gopls exceeds a ceiling (MB)
//   - fd      — system-wide FD-table usage (kern.num_files / kern.maxfiles)
//     exceeds a fraction (default 0.80), i.e. FD-exhaustion risk
//
// Crucially, remediation is NOT "kill every gopls". Every healthy session runs
// its own gopls, so a raw count over-reports (ce-u7v9) and a blind `pkill gopls`
// would tear down live sessions' language servers. The only provably-safe
// reap target is an ORPHAN (PPID==1): a gopls whose parent has died. The
// watchdog therefore alarms on the totals but remediates only orphans — see
// the OrphanCount field and the gopls-watchdog command, which delegates the
// kill to the existing orphan reaper.
package goplswatch

import "fmt"

// orphanPPID is the parent PID a process is reparented to once its real parent
// dies. On both Linux and macOS this is PID 1 (init / launchd).
const orphanPPID = 1

// Proc is a single observed gopls process.
type Proc struct {
	PID    int    `json:"pid"`
	PPID   int    `json:"ppid"`
	RSSKiB int64  `json:"rss_kib"` // resident set size, kibibytes (ps `rss` column)
	Cmd    string `json:"cmd"`     // executable basename, for diagnostics
}

// IsOrphan reports whether this gopls has been reparented to PID 1 — the
// provable signal that no live session owns it, and the only safe reap target.
func (p Proc) IsOrphan() bool { return p.PPID == orphanPPID }

// Sample is a point-in-time measurement of gopls pressure.
type Sample struct {
	// Procs are the individual gopls processes observed this pass.
	Procs []Proc `json:"procs"`

	// FDFraction is system-wide FD-table usage in [0,1]
	// (kern.num_files / kern.maxfiles on Darwin). Zero when unavailable.
	FDFraction float64 `json:"fd_fraction"`
}

// Count returns the total number of gopls processes.
func (s Sample) Count() int { return len(s.Procs) }

// OrphanCount returns the number of orphaned (PPID==1) gopls processes — the
// subset the watchdog is allowed to reap.
func (s Sample) OrphanCount() int {
	n := 0
	for _, p := range s.Procs {
		if p.IsOrphan() {
			n++
		}
	}
	return n
}

// RSSKiB returns the summed resident set size of all gopls processes, in KiB.
func (s Sample) RSSKiB() int64 {
	var sum int64
	for _, p := range s.Procs {
		sum += p.RSSKiB
	}
	return sum
}

// RSSMB returns the summed resident set size of all gopls processes, in MB
// (1 MB == 1024 KiB here, matching the operator-facing "MB" ceiling).
func (s Sample) RSSMB() float64 { return float64(s.RSSKiB()) / 1024.0 }

// OrphanRSSKiB returns the summed resident set size of orphaned gopls only —
// the memory the watchdog can actually reclaim by reaping.
func (s Sample) OrphanRSSKiB() int64 {
	var sum int64
	for _, p := range s.Procs {
		if p.IsOrphan() {
			sum += p.RSSKiB
		}
	}
	return sum
}

// OrphanRSSMB returns OrphanRSSKiB in MB.
func (s Sample) OrphanRSSMB() float64 { return float64(s.OrphanRSSKiB()) / 1024.0 }

// Thresholds are the ceilings above which the watchdog alarms. A non-positive
// field disables that check.
//
// The count and rss ceilings deliberately apply to the ORPHANED gopls subset,
// not the raw total. Every healthy Claude Code session runs its own gopls, so a
// raw count/RSS scales with the number of live sessions and produces phantom
// alarms (ce-u7v9) — and the watchdog cannot safely kill a live session's gopls
// anyway. Orphans (PPID==1) are both the actual leak signal (the 2026-06-15 P0
// was 14+ orphaned gopls at ~7GB) and the only thing the watchdog can reclaim.
// FD-table usage is the one genuinely system-wide signal and stays a total.
type Thresholds struct {
	// MaxCount alarms when the ORPHANED gopls count exceeds this value.
	MaxCount int `json:"max_count"`
	// MaxRSSMB alarms when summed ORPHANED gopls RSS (MB) exceeds this value.
	MaxRSSMB float64 `json:"max_rss_mb"`
	// MaxFDFraction alarms when system-wide FD-table usage exceeds this
	// fraction [0,1].
	MaxFDFraction float64 `json:"max_fd_fraction"`
}

// DefaultThresholds are the ce-710r.3 defaults: more than 3 orphaned gopls,
// more than 2000 MB of orphaned-gopls RSS, or FD-table usage above 80%.
var DefaultThresholds = Thresholds{
	MaxCount:      3,
	MaxRSSMB:      2000,
	MaxFDFraction: 0.80,
}

// withDefaults fills in non-positive fields from DefaultThresholds so a caller
// can override one ceiling without zeroing the others.
func (t Thresholds) withDefaults() Thresholds {
	if t.MaxCount <= 0 {
		t.MaxCount = DefaultThresholds.MaxCount
	}
	if t.MaxRSSMB <= 0 {
		t.MaxRSSMB = DefaultThresholds.MaxRSSMB
	}
	if t.MaxFDFraction <= 0 {
		t.MaxFDFraction = DefaultThresholds.MaxFDFraction
	}
	return t
}

// Alarm is one breached threshold.
type Alarm struct {
	Metric    string  `json:"metric"`    // "count" | "rss" | "fd"
	Value     float64 `json:"value"`     // observed value
	Threshold float64 `json:"threshold"` // ceiling that was exceeded
	Message   string  `json:"message"`   // human-readable one-liner
}

// Evaluate compares a sample against thresholds and returns the breached
// metrics (empty when all are within limits). Non-positive threshold fields
// fall back to DefaultThresholds.
func Evaluate(s Sample, t Thresholds) []Alarm {
	t = t.withDefaults()
	var alarms []Alarm

	if c := s.OrphanCount(); c > t.MaxCount {
		alarms = append(alarms, Alarm{
			Metric: "count", Value: float64(c), Threshold: float64(t.MaxCount),
			Message: fmt.Sprintf("orphaned gopls count %d exceeds limit %d (%d total)",
				c, t.MaxCount, s.Count()),
		})
	}
	if rss := s.OrphanRSSMB(); rss > t.MaxRSSMB {
		alarms = append(alarms, Alarm{
			Metric: "rss", Value: rss, Threshold: t.MaxRSSMB,
			Message: fmt.Sprintf("orphaned gopls RSS %.0f MB exceeds limit %.0f MB (%.0f MB total)",
				rss, t.MaxRSSMB, s.RSSMB()),
		})
	}
	if s.FDFraction > t.MaxFDFraction {
		alarms = append(alarms, Alarm{
			Metric: "fd", Value: s.FDFraction, Threshold: t.MaxFDFraction,
			Message: fmt.Sprintf("system FD-table usage %.1f%% exceeds limit %.1f%% (FD-exhaustion risk)",
				s.FDFraction*100, t.MaxFDFraction*100),
		})
	}
	return alarms
}
