// Command merge-health reports whether the merge pipeline has produced its
// positive event: at least one commit landing on a tracked ref (default
// origin/main) within a lookback window.
//
// It is a sibling of cmd/jaeger-health and copies its contract on purpose:
// exit 0 healthy / 1 degraded / 2 down / 3 usage, --lookback, --json. The
// 2026-09-01 absence-blindness meta-retro found the fleet did not lack
// absence detectors - jaeger-health implemented exactly the right check
// while OTel sat dark for 46 days - it lacked the layer that runs them and
// consumes their exit codes. Absence checks are therefore small spec-pinned
// binaries with one shared exit contract, registered as command pulses in
// cmd/absence-alarm, which schedules them and escalates their failures.
//
// The check reads the local clone's view of the remote ref and never
// fetches (MH-08). If the fetch loop dies, the ref stops moving and this
// check degrades: a dead fetch loop is itself an absent positive event in
// the same pipeline. The report carries the last-fetch age (MH-07) so a
// responder can tell "nothing merged" from "nothing fetched" at a glance.
//
// Usage:
//
//	merge-health [--repo ~/src/dear-agent] [--ref origin/main] [--lookback 96h] [--json]
//
// Exit codes:
//
//	0  healthy  — a commit landed on the ref inside the lookback window
//	1  degraded — ref resolves but its tip is older than the lookback window
//	2  down     — repository or ref cannot be evaluated
//	3  usage    — bad flags
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
	"strconv"
	"strings"
	"time"
)

// clockSkewTolerance bounds how far in the future a commit time may sit
// before it stops counting as proof of a live pipeline (MH-05).
const clockSkewTolerance = 5 * time.Minute

// Report is the machine-readable output emitted with --json (MH-06).
type Report struct {
	CheckedAt    string `json:"checked_at"`
	Repo         string `json:"repo"`
	Ref          string `json:"ref"`
	Status       string `json:"status"` // "healthy" | "degraded" | "down"
	TipCommit    string `json:"tip_commit,omitempty"`
	TipTime      string `json:"tip_time,omitempty"`
	TipAge       string `json:"tip_age,omitempty"`
	Lookback     string `json:"lookback"`
	LastFetchAge string `json:"last_fetch_age,omitempty"`
	Error        string `json:"error,omitempty"`
}

// deps are the injectable host observations so tests never shell out.
type deps struct {
	now        func() time.Time
	gitOutput  func(ctx context.Context, repo string, args ...string) (string, error)
	fetchMtime func(repo string) (time.Time, bool)
}

func defaultDeps() deps {
	return deps{
		now: time.Now,
		gitOutput: func(ctx context.Context, repo string, args ...string) (string, error) {
			full := append([]string{"-C", repo}, args...)
			out, err := exec.CommandContext(ctx, "git", full...).Output()
			if err != nil {
				if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && len(exitErr.Stderr) > 0 {
					return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
				}
				return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
			}
			return strings.TrimSpace(string(out)), nil
		},
		fetchMtime: func(repo string) (time.Time, bool) {
			gitDir, err := exec.Command("git", "-C", repo, "rev-parse", "--git-common-dir").Output()
			if err != nil {
				return time.Time{}, false
			}
			dir := strings.TrimSpace(string(gitDir))
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(repo, dir)
			}
			fi, err := os.Stat(filepath.Join(dir, "FETCH_HEAD"))
			if err != nil {
				return time.Time{}, false
			}
			return fi.ModTime(), true
		},
	}
}

func main() { os.Exit(run(os.Args[1:], defaultDeps())) }

func run(args []string, d deps) int {
	fs := flag.NewFlagSet("merge-health", flag.ContinueOnError)
	home, _ := os.UserHomeDir()
	repo := fs.String("repo", filepath.Join(home, "src", "dear-agent"), "repository to inspect")
	ref := fs.String("ref", "origin/main", "ref whose tip must have moved inside the lookback window")
	lookback := fs.String("lookback", "96h", "maximum silence window (e.g. 24h, 96h)")
	asJSON := fs.Bool("json", false, "emit a JSON report to stdout instead of a human summary")
	if err := fs.Parse(args); err != nil {
		return 3
	}

	window, err := time.ParseDuration(*lookback)
	if err != nil || window <= 0 {
		fmt.Fprintf(os.Stderr, "merge-health: invalid --lookback %q\n", *lookback)
		return 3
	}

	now := d.now()
	r := Report{
		CheckedAt: now.UTC().Format(time.RFC3339),
		Repo:      *repo,
		Ref:       *ref,
		Lookback:  *lookback,
	}
	if mtime, ok := d.fetchMtime(*repo); ok {
		r.LastFetchAge = now.Sub(mtime).Round(time.Minute).String()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// MH-03: an unreadable repo or unresolvable ref is "down", not silence.
	out, err := d.gitOutput(ctx, *repo, "log", "-1", "--format=%H %ct", *ref)
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, *asJSON, fmt.Sprintf("DOWN — cannot read %s in %s: %v", *ref, *repo, err), 2)
	}
	hash, epoch, err := parseTip(out)
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, *asJSON, fmt.Sprintf("DOWN — unexpected git output for %s: %v", *ref, err), 2)
	}

	tipTime := time.Unix(epoch, 0)
	r.TipCommit = hash
	r.TipTime = tipTime.UTC().Format(time.RFC3339)
	age := now.Sub(tipTime)
	r.TipAge = age.Round(time.Minute).String()

	// MH-05: a future commit time is not proof of a live pipeline.
	if tipTime.After(now.Add(clockSkewTolerance)) {
		r.Status = "down"
		r.Error = "tip commit time is in the future"
		return emit(r, *asJSON, fmt.Sprintf("DOWN — %s tip %s is %s in the future", *ref, shortHash(hash), (-age).Round(time.Second)), 2)
	}

	if age > window {
		r.Status = "degraded"
		msg := fmt.Sprintf("DEGRADED — no commit on %s in last %s (tip %s is %s old%s)",
			*ref, *lookback, shortHash(hash), r.TipAge, fetchNote(r.LastFetchAge))
		return emit(r, *asJSON, msg, 1)
	}

	r.Status = "healthy"
	msg := fmt.Sprintf("HEALTHY — %s tip %s landed %s ago (window %s)", *ref, shortHash(hash), r.TipAge, *lookback)
	return emit(r, *asJSON, msg, 0)
}

// parseTip splits "<hash> <unix-epoch>" from git log --format=%H %ct.
func parseTip(out string) (string, int64, error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", 0, fmt.Errorf("want %q, got %q", "<hash> <epoch>", out)
	}
	epoch, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("bad epoch %q: %w", fields[1], err)
	}
	return fields[0], epoch, nil
}

func shortHash(h string) string {
	if len(h) > 9 {
		return h[:9]
	}
	return h
}

// fetchNote renders the advisory last-fetch age (MH-07) so a responder can
// tell "nothing merged" from "nothing fetched".
func fetchNote(fetchAge string) string {
	if fetchAge == "" {
		return ""
	}
	return fmt.Sprintf("; last fetch %s ago", fetchAge)
}

func emit(r Report, asJSON bool, msg string, code int) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(os.Stderr, "merge-health: encode report: %v\n", err)
		}
		return code
	}
	fmt.Println(msg)
	return code
}
