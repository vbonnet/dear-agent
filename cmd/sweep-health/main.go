// Command sweep-health reports whether the sandbox garbage collector has produced
// its positive event: at least one proof-of-completed-sweep record within a
// configured lookback window (default 6h, matching DW-17).
//
// It is a sibling of cmd/bead-health, cmd/merge-health, and cmd/jaeger-health,
// adhering to the shared absence-alarm exit-code contract:
//
//	0  healthy  - at least one completed sweep inside lookback
//	1  degraded - log is readable but no completed sweep inside lookback
//	2  down     - log file cannot be evaluated or clock skew exceeded
//	3  usage    - bad flags or invalid lookback
//
// Usage:
//
//	sweep-health [--log ~/.agm/logs/gc.jsonl] [--lookback 6h] [--json]
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	clockSkewTolerance   = 5 * time.Minute
	maxLogRecordBytes    = 1024 * 1024
	maxLogScanBytes      = int64(8 * 1024 * 1024)
	gcCompletedOperation = "sandbox_gc_completed"
	gcOperationPrefix    = "sandbox_gc"
)

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

// gcEntry captures the log fields checked for reaper liveness.
type gcEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	Operation     string    `json:"operation"`
	Source        string    `json:"source,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	DryRun        bool      `json:"dry_run,omitempty"`
	Errors        int       `json:"errors,omitempty"`
	ProbeFailures int       `json:"probe_failures,omitempty"`
}

func (e gcEntry) isValidCompletedSweep() bool {
	return e.Operation == gcCompletedOperation && !e.DryRun && e.Errors == 0 && e.ProbeFailures == 0
}

func rejectedReason(e gcEntry) string {
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

type logSummary struct {
	hasCompletion         bool
	latestCompletedAt     time.Time
	latestCompletedReason string
	latestRejectedAt      time.Time
	latestRejectedReason  string
	latestReapAt          time.Time
	latestFutureAt        time.Time
}

type deps struct {
	now         func() time.Time
	userHomeDir func() (string, error)
	statFile    func(path string) (os.FileInfo, error)
	openFile    func(path string) (*os.File, error)
}

func defaultDeps() deps {
	return deps{
		now:         time.Now,
		userHomeDir: os.UserHomeDir,
		statFile:    os.Stat,
		openFile:    os.Open,
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

	return cliConfig{
		logPath:  resolvedLog,
		lookback: *lookback,
		window:   window,
		asJSON:   *asJSON,
	}, 0
}

func run(args []string, d deps) int {
	cfg, code := parseCLIArgs(args, d)
	if code != 0 {
		return code
	}

	now := d.now()
	r := Report{
		CheckedAt: now.UTC().Format(time.RFC3339),
		Log:       cfg.logPath,
		Lookback:  cfg.lookback,
	}

	statFile := d.statFile
	if statFile == nil {
		statFile = os.Stat
	}
	fi, err := statFile(cfg.logPath)
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: cannot access sandbox GC log in %s: %v", cfg.logPath, err), 2)
	}
	if fi.IsDir() {
		r.Status = "down"
		r.Error = "log path is a directory"
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: sandbox GC log %s is a directory", cfg.logPath), 2)
	}

	openFile := d.openFile
	if openFile == nil {
		openFile = os.Open
	}
	f, err := openFile(cfg.logPath)
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: cannot open sandbox GC log in %s: %v", cfg.logPath, err), 2)
	}
	defer f.Close()

	summary, err := scanLog(f, fi.Size())
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: error reading sandbox GC log in %s: %v", cfg.logPath, err), 2)
	}

	msg, exitCode := evaluateSweep(summary, now, cfg.window, cfg.lookback, &r)
	return emit(r, cfg.asJSON, msg, exitCode)
}

func scanLog(f *os.File, size int64) (logSummary, error) {
	var summary logSummary
	if size > maxLogScanBytes {
		if _, err := f.Seek(size-maxLogScanBytes, io.SeekStart); err == nil {
			// Discard the initial partial record
			skip := bufio.NewReader(f)
			if _, err := skip.ReadBytes('\n'); err != nil && !errors.Is(err, io.EOF) {
				return summary, err
			}
		}
	}

	reader := bufio.NewReaderSize(f, maxLogRecordBytes)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var e gcEntry
			if json.Unmarshal(line, &e) == nil && strings.HasPrefix(e.Operation, gcOperationPrefix) {
				foldRecord(&summary, e)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return summary, readErr
		}
	}
	return summary, nil
}

func foldRecord(s *logSummary, e gcEntry) {
	if e.Timestamp.IsZero() {
		return
	}
	switch {
	case e.isValidCompletedSweep():
		s.hasCompletion = true
		if e.Timestamp.After(s.latestCompletedAt) {
			s.latestCompletedAt = e.Timestamp
			s.latestCompletedReason = e.Reason
		}
	case e.Operation == gcCompletedOperation:
		s.hasCompletion = true
		if e.Timestamp.After(s.latestRejectedAt) {
			s.latestRejectedAt = e.Timestamp
			s.latestRejectedReason = rejectedReason(e)
		}
	case e.Operation == "sandbox_gc_reap":
		if e.Timestamp.After(s.latestReapAt) {
			s.latestReapAt = e.Timestamp
		}
	}
	if e.Timestamp.After(s.latestFutureAt) {
		s.latestFutureAt = e.Timestamp
	}
}

func evaluateSweep(s logSummary, now time.Time, window time.Duration, lookback string, r *Report) (string, int) {
	// SWEEP-05: check clock skew
	if !s.latestFutureAt.IsZero() && s.latestFutureAt.After(now.Add(clockSkewTolerance)) {
		age := now.Sub(s.latestFutureAt)
		r.Status = "down"
		r.Error = "latest sweep timestamp is in the future"
		return fmt.Sprintf("DOWN: latest sweep timestamp %s is %s in the future",
			s.latestFutureAt.UTC().Format(time.RFC3339), (-age).Round(time.Second)), 2
	}

	targetTime := s.latestCompletedAt
	if targetTime.IsZero() {
		if s.hasCompletion && !s.latestRejectedAt.IsZero() {
			r.Status = "degraded"
			r.Error = s.latestRejectedReason
			return fmt.Sprintf("DEGRADED: %s", s.latestRejectedReason), 1
		}
		// Compatibility fallback if log predates sandbox_gc_completed
		if !s.hasCompletion && !s.latestReapAt.IsZero() {
			targetTime = s.latestReapAt
			r.Reason = "liveness inferred from reap records (log predates sandbox_gc_completed heartbeat)"
		} else {
			r.Status = "degraded"
			r.Error = "no completed sandbox sweeps found in log"
			return fmt.Sprintf("DEGRADED: no completed sandbox sweeps found in %s", r.Log), 1
		}
	}

	age := now.Sub(targetTime)
	r.LatestSweepAt = targetTime.UTC().Format(time.RFC3339)
	r.LatestSweepAge = age.Round(time.Minute).String()

	if age > window {
		r.Status = "degraded"
		if !s.latestRejectedAt.IsZero() && s.latestRejectedAt.After(targetTime) {
			r.Error = s.latestRejectedReason
			return fmt.Sprintf("DEGRADED: no completed sweep in last %s; latest sweep was rejected: %s", lookback, s.latestRejectedReason), 1
		}
		msg := fmt.Sprintf("DEGRADED: no sandbox sweep completed in last %s (latest completed %s ago)", lookback, r.LatestSweepAge)
		return msg, 1
	}

	r.Status = "healthy"
	msg := fmt.Sprintf("HEALTHY: sandbox sweep completed %s ago (window %s)", r.LatestSweepAge, lookback)
	return msg, 0
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
