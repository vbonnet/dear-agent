// Command bead-health reports whether the Beads project store has produced its
// positive event: at least one bead closed within a lookback window.
//
// It is a sibling of cmd/jaeger-health and cmd/merge-health and copies their
// contract on purpose: exit 0 healthy / 1 degraded / 2 down / 3 usage, --lookback,
// and --json. Absence checks are small spec-pinned binaries with one shared exit
// contract, registered as command pulses in cmd/absence-alarm, which schedules
// them and escalates their failures.
//
// The check queries the canonical Beads store in read-only mode (--readonly)
// and never mutates the database (BH-08).
//
// Usage:
//
//	bead-health [--db ~/beads/context-engine/.beads] [--lookback 48h] [--json]
//
// Exit codes:
//
//	0  healthy  - at least one bead closed inside the lookback window
//	1  degraded - database accessible but no bead closed inside the lookback window
//	2  down     - database cannot be read, query failed, or clock skew exceeded
//	3  usage    - bad flags
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// clockSkewTolerance bounds how far in the future a closure time may sit
// before it stops counting as proof of a live pipeline (BH-05).
const clockSkewTolerance = 5 * time.Minute

// Report is the machine-readable output emitted with --json (BH-07).
type Report struct {
	CheckedAt       string `json:"checked_at"`
	DB              string `json:"db"`
	Status          string `json:"status"` // "healthy" | "degraded" | "down"
	LatestID        string `json:"latest_id,omitempty"`
	LatestTitle     string `json:"latest_title,omitempty"`
	LatestClosedAt  string `json:"latest_closed_at,omitempty"`
	LatestClosedAge string `json:"latest_closed_age,omitempty"`
	Lookback        string `json:"lookback"`
	Error           string `json:"error,omitempty"`
}

// BeadRecord captures the minimal issue fields required for closure health checks.
type BeadRecord struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	ClosedAt string `json:"closed_at"`
}

// deps are injectable host observations so tests never require a real database.
type deps struct {
	now               func() time.Time
	userHomeDir       func() (string, error)
	statDB            func(path string) (os.FileInfo, error)
	queryLatestClosed func(ctx context.Context, db string) ([]BeadRecord, error)
}

func defaultDeps() deps {
	return deps{
		now:         time.Now,
		userHomeDir: os.UserHomeDir,
		statDB:      os.Stat,
		queryLatestClosed: func(ctx context.Context, db string) ([]BeadRecord, error) {
			args := []string{
				"--db", db,
				"--readonly",
				"list",
				"--status", "closed",
				"--sort", "closed",
				"--limit", "1",
				"--json",
			}
			out, err := exec.CommandContext(ctx, "bd", args...).Output()
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
					return nil, fmt.Errorf("bd: %s", strings.TrimSpace(string(exitErr.Stderr)))
				}
				return nil, fmt.Errorf("exec bd: %w", err)
			}
			var records []BeadRecord
			if err := json.Unmarshal(out, &records); err != nil {
				return nil, fmt.Errorf("unmarshal bd output: %w", err)
			}
			return records, nil
		},
	}
}

func main() { os.Exit(run(os.Args[1:], defaultDeps())) }

type cliConfig struct {
	db       string
	lookback string
	window   time.Duration
	asJSON   bool
}

func parseCLIArgs(args []string, d deps) (cliConfig, int) {
	fs := flag.NewFlagSet("bead-health", flag.ContinueOnError)
	userHome := d.userHomeDir
	if userHome == nil {
		userHome = os.UserHomeDir
	}
	home, homeErr := userHome()
	defaultDB := os.Getenv("BEADS_DB")
	if defaultDB == "" && homeErr == nil {
		defaultDB = filepath.Join(home, "beads", "context-engine", ".beads")
	}
	db := fs.String("db", defaultDB, "path to canonical beads database directory")
	lookback := fs.String("lookback", "48h", "maximum silence window (e.g. 24h, 48h)")
	asJSON := fs.Bool("json", false, "emit a JSON report to stdout instead of a human summary")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, 3
	}

	window, err := time.ParseDuration(*lookback)
	if err != nil || window <= 0 {
		fmt.Fprintf(os.Stderr, "bead-health: invalid --lookback %q\n", *lookback)
		return cliConfig{}, 3
	}

	resolvedDB := *db
	if strings.HasPrefix(resolvedDB, "~/") {
		if homeErr != nil {
			fmt.Fprintf(os.Stderr, "bead-health: cannot resolve home directory: %v\n", homeErr)
			return cliConfig{}, 3
		}
		resolvedDB = filepath.Join(home, resolvedDB[2:])
	}

	return cliConfig{
		db:       resolvedDB,
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

	// BH-09: One deadline covers subprocesses run by this probe. bead-health is
	// scheduled by absence-alarm, so a query that hangs must report a bounded
	// failure rather than stalling the observation scheduler.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := d.now()
	r := Report{
		CheckedAt: now.UTC().Format(time.RFC3339),
		DB:        cfg.db,
		Lookback:  cfg.lookback,
	}

	statDB := d.statDB
	if statDB == nil {
		statDB = os.Stat
	}
	if fi, err := statDB(cfg.db); err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: cannot access beads database in %s: %v", cfg.db, err), 2)
	} else if !fi.IsDir() {
		r.Status = "down"
		r.Error = "beads path is not a directory"
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: beads path %s is not a directory", cfg.db), 2)
	}

	records, err := d.queryLatestClosed(ctx, cfg.db)
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, cfg.asJSON, fmt.Sprintf("DOWN: cannot query beads in %s: %v", cfg.db, err), 2)
	}

	if len(records) == 0 {
		r.Status = "degraded"
		r.Error = "no closed beads in database"
		return emit(r, cfg.asJSON, fmt.Sprintf("DEGRADED: no closed beads found in %s", cfg.db), 1)
	}

	latest := records[0]
	r.LatestID = latest.ID
	r.LatestTitle = latest.Title
	r.LatestClosedAt = latest.ClosedAt

	msg, exitCode := evaluateClosure(latest, now, cfg.window, cfg.lookback, &r)
	return emit(r, cfg.asJSON, msg, exitCode)
}

func evaluateClosure(latest BeadRecord, now time.Time, window time.Duration, lookback string, r *Report) (string, int) {
	if latest.ClosedAt == "" {
		r.Status = "degraded"
		r.Error = fmt.Sprintf("latest closed bead %s has empty closed_at", latest.ID)
		return fmt.Sprintf("DEGRADED: latest closed bead %s has no closure timestamp", latest.ID), 1
	}

	closedTime, err := parseClosedTime(latest.ClosedAt)
	if err != nil {
		r.Status = "down"
		r.Error = fmt.Sprintf("parse closure time: %v", err)
		return fmt.Sprintf("DOWN: cannot parse closed_at for %s: %v", latest.ID, err), 2
	}

	// BH-05: a future closure time is not proof of a live pipeline.
	if closedTime.After(now.Add(clockSkewTolerance)) {
		age := now.Sub(closedTime)
		r.Status = "down"
		r.Error = "closure timestamp is in the future"
		return fmt.Sprintf("DOWN: latest bead %s closure time %s is %s in the future", latest.ID, latest.ClosedAt, (-age).Round(time.Second)), 2
	}

	age := now.Sub(closedTime)
	r.LatestClosedAge = age.Round(time.Minute).String()

	if age > window {
		r.Status = "degraded"
		msg := fmt.Sprintf("DEGRADED: no bead closed in last %s (latest %s %q closed %s ago)", lookback, latest.ID, latest.Title, r.LatestClosedAge)
		return msg, 1
	}

	r.Status = "healthy"
	msg := fmt.Sprintf("HEALTHY: bead %s %q closed %s ago (window %s)", latest.ID, latest.Title, r.LatestClosedAge, lookback)
	return msg, 0
}

func parseClosedTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func emit(r Report, asJSON bool, msg string, code int) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(os.Stderr, "bead-health: encode report: %v\n", err)
		}
		return code
	}
	fmt.Println(msg)
	return code
}
