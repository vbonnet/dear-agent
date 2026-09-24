// Command sweep-health reports whether the scheduled sandbox reaper produced
// a completed sweep inside the lookback window. Exit 0 means healthy, 1
// degraded, 2 down, and 3 usage error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/gcloghealth"
)

const maxLogScanBytes = gcloghealth.DefaultInitialBytes

// Report is the machine-readable output emitted with --json (SWEEP-07).
type Report struct {
	CheckedAt      string `json:"checked_at"`
	Log            string `json:"log"`
	Status         string `json:"status"` // "healthy" | "degraded" | "down"
	LatestSweepAt  string `json:"latest_sweep_at,omitempty"`
	LatestSweepAge string `json:"latest_sweep_age,omitempty"`
	Lookback       string `json:"lookback"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
}

type deps struct {
	now             func() time.Time
	userHomeDir     func() (string, error)
	maxLogScanBytes int64
	maxLogMaxBytes  int64
}

func defaultDeps() deps {
	return deps{
		now:             time.Now,
		userHomeDir:     os.UserHomeDir,
		maxLogScanBytes: maxLogScanBytes,
		maxLogMaxBytes:  gcloghealth.DefaultMaxBytes,
	}
}

type cliConfig struct {
	logPath  string
	lookback string
	window   time.Duration
	asJSON   bool
}

func main() { os.Exit(run(os.Args[1:], defaultDeps())) }

func parseCLIArgs(args []string, d deps) (cliConfig, int) {
	fs := flag.NewFlagSet("sweep-health", flag.ContinueOnError)
	userHome := d.userHomeDir
	if userHome == nil {
		userHome = os.UserHomeDir
	}
	home, homeErr := userHome()
	defaultLog := os.Getenv("AGM_GC_LOG")
	if defaultLog == "" && homeErr == nil {
		defaultLog = filepath.Join(home, ".agm", "logs", "gc.jsonl")
	}
	logFlag := fs.String("log", defaultLog, "path to sandbox GC log file (gc.jsonl)")
	lookback := fs.String("lookback", "6h", "maximum silence window (e.g. 2h, 6h)")
	asJSON := fs.Bool("json", false, "emit a JSON report to stdout instead of a human summary")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, 3
	}
	window, err := time.ParseDuration(*lookback)
	if err != nil || window <= 0 {
		fmt.Fprintf(os.Stderr, "sweep-health: invalid --lookback %q\n", *lookback)
		return cliConfig{}, 3
	}
	resolvedLog := *logFlag
	if strings.HasPrefix(resolvedLog, "~/") {
		if homeErr != nil {
			fmt.Fprintf(os.Stderr, "sweep-health: cannot resolve home directory: %v\n", homeErr)
			return cliConfig{}, 3
		}
		resolvedLog = filepath.Join(home, resolvedLog[2:])
	}
	return cliConfig{logPath: resolvedLog, lookback: *lookback, window: window, asJSON: *asJSON}, 0
}

func run(args []string, d deps) int {
	cfg, code := parseCLIArgs(args, d)
	if code != 0 {
		return code
	}
	now := d.now()
	r := Report{CheckedAt: now.UTC().Format(time.RFC3339), Log: cfg.logPath, Lookback: cfg.lookback}
	fi, err := os.Stat(cfg.logPath)
	if err != nil {
		r.Status, r.Error = "down", err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: cannot access sandbox GC log in %s: %v", cfg.logPath, err), 2)
	}
	if fi.IsDir() {
		r.Status, r.Error = "down", "log path is a directory"
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: sandbox GC log %s is a directory", cfg.logPath), 2)
	}
	summary, err := gcloghealth.Scan(cfg.logPath, gcloghealth.Options{
		Now:            now,
		MaxAge:         cfg.window,
		InitialBytes:   d.maxLogScanBytes,
		MaxBytes:       d.maxLogMaxBytes,
		IgnoredSources: []string{gcloghealth.WatchdogSource},
	})
	if err != nil {
		r.Status, r.Error = "down", err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: error reading sandbox GC log in %s: %v", cfg.logPath, err), 2)
	}
	msg, code := evaluateSweep(summary, now, cfg.window, cfg.lookback, &r)
	return emit(r, cfg.asJSON, msg, code)
}

func evaluateSweep(s gcloghealth.Summary, now time.Time, window time.Duration, lookback string, r *Report) (string, int) {
	if !s.LastFutureCompletionAt.IsZero() {
		age := now.Sub(s.LastFutureCompletionAt)
		r.Status, r.Error = "down", "latest sweep timestamp is in the future"
		return fmt.Sprintf("DOWN: latest sweep timestamp %s is %s in the future",
			s.LastFutureCompletionAt.UTC().Format(time.RFC3339), (-age).Round(time.Second)), 2
	}
	targetTime, viaFallback := s.Proof()
	if targetTime.IsZero() {
		r.Status = "degraded"
		switch {
		case s.Indeterminate:
			// A capped scan can observe a real error and still be unable to
			// rule out older history. Reporting only the uncertainty threw
			// away the one actionable thing the tail did contain, so an
			// operator saw "we did not look far enough" instead of the dry
			// run, deletion, probe, or GC failure that was right there.
			r.Error = "sandbox GC liveness is undetermined: older history was not scanned"
			// Order the error against the observed success, the way the
			// watchdog adapter does. Proof() is zero for an indeterminate
			// scan, so an error the log itself shows was followed by a
			// successful sweep would otherwise be reported as the live
			// problem and send responders after something already fixed.
			if s.LastError != "" && s.LastErrorAt.After(s.LastSuccess) {
				r.Error += "; most recent observed error: " + s.LastError
			}
			return "DEGRADED: " + r.Error, 1
		case s.LastError != "" && s.LastErrorAt.After(s.LastSuccess):
			r.Error = s.LastError
			return "DEGRADED: " + s.LastError, 1
		default:
			r.Error = "no completed sandbox sweeps found in log"
			return fmt.Sprintf("DEGRADED: no completed sandbox sweeps found in %s", r.Log), 1
		}
	}
	if viaFallback {
		r.Reason = "liveness inferred from reap records (log predates sandbox_gc_completed heartbeat)"
	}
	age := now.Sub(targetTime)
	r.LatestSweepAt = targetTime.UTC().Format(time.RFC3339)
	r.LatestSweepAge = age.Round(time.Minute).String()
	if age > window {
		r.Status = "degraded"
		if s.LastErrorAt.After(targetTime) {
			r.Error = s.LastError
			return fmt.Sprintf("DEGRADED: no completed sweep in last %s; latest sweep was rejected: %s", lookback, s.LastError), 1
		}
		return fmt.Sprintf("DEGRADED: no sandbox sweep completed in last %s (latest completed %s ago)", lookback, r.LatestSweepAge), 1
	}
	r.Status = "healthy"
	return fmt.Sprintf("HEALTHY: sandbox sweep completed %s ago (window %s)", r.LatestSweepAge, lookback), 0
}

func emit(r Report, asJSON bool, msg string, code int) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(os.Stderr, "sweep-health: encode report: %v\n", err)
		}
		return code
	}
	fmt.Println(msg)
	return code
}
