// Command burndown-maint is the host-side bead-burndown maintenance tick
// (ce-cd14.2, Phase B of ce-cd14). It replaces the Cowork-sandbox
// bead-burndown-loop skill with a host loop that:
//
//  1. Counts active burndown worker sessions via `agm session list --json`
//  2. Spawns at most 1 new worker per tick if below the target (default N=1)
//  3. Creates a worktree + AGM session for the new worker
//
// Designed to run under agm-job for locking and escalation:
//
//	agm-job run --name burndown-maint \
//	    --verify 'agm session list --json | jq -e ".sessions | length > 0"' \
//	    --escalate-session vroom-orchestrator \
//	    -- burndown-maint
//
// Register as a host loop:
//
//	agm loop new burndown-maint --cadence 2h \
//	    --cmd 'agm-job run --name burndown-maint --verify "true" -- burndown-maint' \
//	    --description "Maintain N=1 concurrent bead-burndown workers"
//
// Exit codes:
//
//	0  success (no spawn needed, or spawn succeeded)
//	1  failure (spawn failed or session listing failed)
//	2  usage error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/burndownmaint"
)

const (
	exitSuccess       = 0
	exitFailure       = 1
	exitUsage         = 2
	agmCommandTimeout = 30 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "burndown-maint: %v\n", err)
		var code int
		switch {
		case strings.Contains(err.Error(), "usage:"):
			code = exitUsage
		default:
			code = exitFailure
		}
		os.Exit(code)
	}
}

type options struct {
	target    int
	harness   string
	model     string
	workspace string
	jsonOut   bool
	dryRun    bool
}

func run(ctx context.Context, argv []string) error {
	fs := flag.NewFlagSet("burndown-maint", flag.ContinueOnError)
	var opts options
	fs.IntVar(&opts.target, "target", 1, "Desired concurrent burndown workers")
	fs.StringVar(&opts.harness, "harness", "claude-code", "AGM harness for new workers")
	fs.StringVar(&opts.model, "model", "claude-opus-4-8", "Model for new workers")
	fs.StringVar(&opts.workspace, "workspace", "oss", "AGM workspace for new sessions")
	fs.BoolVar(&opts.jsonOut, "json", false, "Output JSON status")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Report what would happen without spawning")

	if err := fs.Parse(argv); err != nil {
		return fmt.Errorf("usage: %w", err)
	}

	ts := time.Now().Format("2006-01-02T15:04:05Z07:00")
	logf := func(format string, a ...any) {
		fmt.Printf("[%s] %s\n", ts, fmt.Sprintf(format, a...))
	}

	active, err := countActiveBurndownWorkers(ctx)
	if err != nil {
		return fmt.Errorf("count active workers: %w", err)
	}
	logf("active burndown workers: %d (target: %d)", active, opts.target)

	result := tickResult{
		Active:  active,
		Target:  opts.target,
		Spawned: 0,
	}

	if active >= opts.target {
		logf("at or above target — no spawn needed")
		return outputResult(result, opts.jsonOut)
	}

	if opts.dryRun {
		logf("dry-run: would spawn 1 worker (harness=%s model=%s)", opts.harness, opts.model)
		result.DryRun = true
		return outputResult(result, opts.jsonOut)
	}

	logf("below target, spawning 1 worker (harness=%s model=%s)", opts.harness, opts.model)
	sessionName := fmt.Sprintf("burndown-%s", time.Now().Format("20060102-150405"))
	sid, err := spawnWorker(ctx, sessionName, burndownmaint.Route{
		Harness: opts.harness, Model: opts.model, Workspace: opts.workspace,
	})
	if err != nil {
		return fmt.Errorf("spawn worker: %w", err)
	}
	logf("spawned session %q (id=%s)", sessionName, sid)
	result.Spawned = 1
	result.SpawnedSession = sid

	return outputResult(result, opts.jsonOut)
}

type tickResult struct {
	Active         int    `json:"active"`
	Target         int    `json:"target"`
	Spawned        int    `json:"spawned"`
	SpawnedSession string `json:"spawned_session,omitempty"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

func outputResult(r tickResult, jsonOut bool) error {
	if !jsonOut {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// countActiveBurndownWorkers shells out to `agm session list --json` and
// counts sessions whose name starts with "burndown-" and are not archived.
func countActiveBurndownWorkers(ctx context.Context) (int, error) {
	out, err := runCommand(ctx, agmCommandTimeout, "agm", "session", "list", "--json")
	if err != nil {
		return 0, fmt.Errorf("agm session list: %w", err)
	}

	var listing struct {
		Sessions []struct {
			Name      string `json:"name"`
			Lifecycle string `json:"lifecycle"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &listing); err != nil {
		return 0, fmt.Errorf("parse session list: %w", err)
	}

	count := 0
	for _, s := range listing.Sessions {
		if strings.HasPrefix(s.Name, "burndown-") && s.Lifecycle != "archived" {
			count++
		}
	}
	return count, nil
}

// spawnWorker creates a new detached AGM session for burndown work.
func spawnWorker(ctx context.Context, name string, route burndownmaint.Route) (string, error) {
	args := burndownmaint.BuildSessionArgs(name, route)
	out, err := runCommand(ctx, agmCommandTimeout, "agm", args...)
	if err != nil {
		return "", fmt.Errorf("agm session new: %w\n%s", err, out)
	}

	return strings.TrimSpace(string(out)), nil
}

func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(timeoutCtx, name, args...).CombinedOutput() //#nosec G204
	if timeoutCtx.Err() != nil {
		return out, timeoutCtx.Err()
	}
	return out, err
}
