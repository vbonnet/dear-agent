// Command gopls-watchdog samples gopls process pressure, alarms when a
// threshold is breached, logs the alarm to the VROOM decision trail, and
// remediates by reaping orphaned gopls.
//
// It is the active half of the ce-710r gopls FD-leak fix: fd-pressure reports
// pressure for humans, the orphan reaper kills orphans on a schedule, and this
// watchdog ties detection to remediation on its own tick. It reaps only orphans
// (PPID==1) — a live session's language server is never touched — so its leak
// alarms (count, RSS) likewise key on the orphaned subset; a busy machine with
// many LIVE gopls is not the leak (ce-u7v9). FD-table usage is system-wide.
//
// Thresholds (ce-710r.3 defaults):
//
//	count : > 3 orphaned gopls processes
//	rss   : > 2000 MB summed orphaned-gopls RSS
//	fd    : > 80% system-wide FD-table usage (kern.num_files / kern.maxfiles)
//
// Usage:
//
//	gopls-watchdog                       # human-readable report; remediate if breached
//	gopls-watchdog --json                # JSON report to stdout
//	gopls-watchdog --dry-run             # detect + log, but do not kill anything
//	gopls-watchdog --max-count 5         # override the count ceiling
//	gopls-watchdog --max-rss-mb 4000     # override the RSS ceiling
//	gopls-watchdog --max-fd-fraction 0.9 # override the FD ceiling
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
	"github.com/vbonnet/dear-agent/pkg/vroom/goplswatch"
)

func main() {
	code, err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gopls-watchdog:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

type config struct {
	jsonOutput bool
	dryRun     bool
	agmBin     string
	trailPath  string
	thresholds goplswatch.Thresholds
}

func run(args []string, out io.Writer) (int, error) {
	fs := flag.NewFlagSet("gopls-watchdog", flag.ContinueOnError)
	fs.SetOutput(out)
	cfg := config{}
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit JSON instead of a human-readable report")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "detect and log, but do not reap any orphans")
	fs.StringVar(&cfg.agmBin, "agm", "agm", "path to the agm binary used for orphan remediation")
	fs.StringVar(&cfg.trailPath, "trail", defaultTrailPath(), "decision-trail JSONL path for alarm records")
	fs.IntVar(&cfg.thresholds.MaxCount, "max-count", goplswatch.DefaultThresholds.MaxCount,
		"alarm when the orphaned gopls count exceeds this value")
	fs.Float64Var(&cfg.thresholds.MaxRSSMB, "max-rss-mb", goplswatch.DefaultThresholds.MaxRSSMB,
		"alarm when summed orphaned-gopls RSS (MB) exceeds this value")
	fs.Float64Var(&cfg.thresholds.MaxFDFraction, "max-fd-fraction", goplswatch.DefaultThresholds.MaxFDFraction,
		"alarm when system FD-table usage exceeds this fraction [0,1]")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sample, err := goplswatch.SampleNow(ctx)
	if err != nil {
		return 2, fmt.Errorf("sample: %w", err)
	}
	alarms := goplswatch.Evaluate(sample, cfg.thresholds)

	var remediation *reapResult
	if len(alarms) > 0 && !cfg.dryRun {
		r, rerr := reapOrphans(ctx, cfg.agmBin)
		if rerr != nil {
			// Remediation failure must not hide the alarm; record it and keep going.
			r = &reapResult{Error: rerr.Error()}
		}
		remediation = r
	}

	// Logging the alarm to the trail is best-effort: a trail write failure is a
	// warning, never a reason to suppress the breach exit code.
	if len(alarms) > 0 {
		if lerr := logAlarm(ctx, cfg, sample, alarms, remediation); lerr != nil {
			fmt.Fprintf(os.Stderr, "gopls-watchdog: warning: trail append failed: %v\n", lerr)
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(out, sample, alarms, remediation, cfg); err != nil {
			return 2, err
		}
	} else {
		emitReport(out, sample, alarms, remediation, cfg)
	}

	if len(alarms) > 0 {
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

// reapResult is the parsed JSON contract of `agm session reap-orphans --json`
// (a subset; field names mirror cmd/agm orphanReapJSON).
type reapResult struct {
	OrphansFound int    `json:"orphans_found"`
	Killed       int    `json:"killed"`
	Failed       int    `json:"failed"`
	KilledPIDs   []int  `json:"killed_pids"`
	Error        string `json:"error,omitempty"`
}

// reapOrphans delegates the actual kill to the canonical orphan reaper, scoped
// to gopls. The reaper only ever signals PPID==1 processes, so this never tears
// down a live session's language server — the safety invariant of ce-710r.
func reapOrphans(ctx context.Context, agmBin string) (*reapResult, error) {
	cmd := exec.CommandContext(ctx, agmBin, "session", "reap-orphans", "--targets", "gopls", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s session reap-orphans: %w", agmBin, err)
	}
	var r reapResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse reap-orphans output: %w", err)
	}
	return &r, nil
}

// logAlarm appends one watchdog.gopls.alarm record to the decision trail.
func logAlarm(ctx context.Context, cfg config, s goplswatch.Sample, alarms []goplswatch.Alarm, rem *reapResult) error {
	trail, err := decisiontrail.OpenJSONL(cfg.trailPath)
	if err != nil {
		return err
	}
	defer trail.Close()

	breached := make([]string, 0, len(alarms))
	messages := make([]string, 0, len(alarms))
	for _, a := range alarms {
		breached = append(breached, a.Metric)
		messages = append(messages, a.Message)
	}

	payload := map[string]any{
		"gopls_count":   s.Count(),
		"orphan_count":  s.OrphanCount(),
		"rss_mb":        s.RSSMB(),
		"orphan_rss_mb": s.OrphanRSSMB(),
		"fd_fraction":   s.FDFraction,
		"breached":      breached,
		"messages":      messages,
		"dry_run":       cfg.dryRun,
		"thresholds": map[string]any{
			"max_count":       cfg.thresholds.MaxCount,
			"max_rss_mb":      cfg.thresholds.MaxRSSMB,
			"max_fd_fraction": cfg.thresholds.MaxFDFraction,
		},
	}
	if rem != nil {
		remediation := map[string]any{
			"orphans_found": rem.OrphansFound,
			"killed":        rem.Killed,
			"failed":        rem.Failed,
			"killed_pids":   rem.KilledPIDs,
		}
		if rem.Error != "" {
			remediation["error"] = rem.Error
		}
		payload["remediation"] = remediation
	}

	return trail.Append(ctx, decisiontrail.Record{
		Role:    "watchdog",
		Kind:    "watchdog.gopls.alarm",
		Payload: payload,
	})
}

func emitJSON(out io.Writer, s goplswatch.Sample, alarms []goplswatch.Alarm, rem *reapResult, cfg config) error {
	type report struct {
		GoplsCount  int                   `json:"gopls_count"`
		OrphanCount int                   `json:"orphan_count"`
		RSSMB       float64               `json:"rss_mb"`
		OrphanRSSMB float64               `json:"orphan_rss_mb"`
		FDFraction  float64               `json:"fd_fraction"`
		Thresholds  goplswatch.Thresholds `json:"thresholds"`
		Alarms      []goplswatch.Alarm    `json:"alarms"`
		Remediation *reapResult           `json:"remediation,omitempty"`
		OK          bool                  `json:"ok"`
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report{
		GoplsCount:  s.Count(),
		OrphanCount: s.OrphanCount(),
		RSSMB:       s.RSSMB(),
		OrphanRSSMB: s.OrphanRSSMB(),
		FDFraction:  s.FDFraction,
		Thresholds:  cfg.thresholds,
		Alarms:      alarms,
		Remediation: rem,
		OK:          len(alarms) == 0,
	})
}

func emitReport(out io.Writer, s goplswatch.Sample, alarms []goplswatch.Alarm, rem *reapResult, cfg config) {
	fmt.Fprintln(out, "gopls-watchdog report")
	fmt.Fprintf(out, "  orphaned gopls  : %d  [limit %d]   (%d total)\n",
		s.OrphanCount(), cfg.thresholds.MaxCount, s.Count())
	fmt.Fprintf(out, "  orphaned RSS    : %.0f MB  [limit %.0f MB]   (%.0f MB total)\n",
		s.OrphanRSSMB(), cfg.thresholds.MaxRSSMB, s.RSSMB())
	fmt.Fprintf(out, "  FD-table usage  : %.1f%%  [limit %.1f%%]\n",
		s.FDFraction*100, cfg.thresholds.MaxFDFraction*100)
	fmt.Fprintln(out)

	if len(alarms) == 0 {
		fmt.Fprintln(out, "Status: OK (all metrics within limits)")
		return
	}
	fmt.Fprintf(out, "Status: ALARM (%d threshold(s) breached)\n", len(alarms))
	for _, a := range alarms {
		fmt.Fprintf(out, "  ! %s\n", a.Message)
	}
	switch {
	case cfg.dryRun:
		fmt.Fprintln(out, "Remediation: skipped (--dry-run)")
	case rem == nil:
		fmt.Fprintln(out, "Remediation: none")
	case rem.Error != "":
		fmt.Fprintf(out, "Remediation: FAILED (%s)\n", rem.Error)
	default:
		fmt.Fprintf(out, "Remediation: reaped %d/%d orphaned gopls", rem.Killed, rem.OrphansFound)
		if rem.Failed > 0 {
			fmt.Fprintf(out, " (%d failed)", rem.Failed)
		}
		fmt.Fprintln(out)
	}
}
