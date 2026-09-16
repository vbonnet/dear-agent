// Command mergeloop is the Ralph Wiggum persistent PR-merge loop (ADR-029): it
// drives every open PR toward MERGED with zero human mechanics — rebasing
// behind branches, spawning agents to fix CI failures and resolve conflicts,
// and delegating the squash-merge to the vetted safe-merge wrapper. The human
// is escalated to only for genuine policy blocks (security / product / money).
//
// Modes:
//
//	mergeloop tick   one idempotent pass over all open PRs (run by launchd/cron)
//	mergeloop run    persistent daemon: tick, sleep --interval, repeat (Ctrl-C to stop)
//
// Usage:
//
//	mergeloop tick [--repo owner/name] [--cap N] [--max-attempts N] [--dry-run] [--enable-agents]
//	               [--rebase-cooldown D]
//	mergeloop run  [--interval 10m] [...same flags]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/mergeloop"
	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nmergeloop: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	mode           string
	repo           string
	interval       time.Duration
	cap            int
	rebaseCooldown time.Duration
	maxAttempts    int
	stallThreshold time.Duration
	dryRun         bool
	enableAgents   bool
	agentHarness   string
	agentModel     string
}

func run(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("usage: mergeloop <tick|run> [flags] (run `mergeloop tick -h` for flags)")
	}
	mode := argv[0]
	if mode == "-h" || mode == "--help" {
		fmt.Println("mergeloop <tick|run> [flags] — persistent PR-merge loop (ADR-029)")
		fmt.Println("  tick   one idempotent pass over all open PRs")
		fmt.Println("  run    daemon: tick, sleep --interval, repeat")
		return nil
	}
	if mode != "tick" && mode != "run" {
		return fmt.Errorf("unknown mode %q (want tick or run)", mode)
	}

	opts := options{mode: mode}
	fs := newMergeLoopFlagSet(mode, &opts)
	if err := fs.Parse(argv[1:]); err != nil {
		return err
	}

	repo := opts.repo
	if repo == "" {
		var err error
		repo, err = detectRepo()
		if err != nil {
			return fmt.Errorf("cannot detect repo: %w (pass --repo owner/name)", err)
		}
	}

	// Telemetry: tracer (spans) + meter (metrics). Both degrade to no-ops when
	// no OTLP endpoint is configured, so this is always safe to call.
	shutdownTracer := otelsetup.InitTracer("mergeloop")
	defer func() {
		if err := shutdownTracer(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "mergeloop: tracer shutdown: %v\n", err)
		}
	}()
	if _, err := telemetry.InitMeter("mergeloop"); err == nil {
		defer func() {
			if err := telemetry.Shutdown(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "mergeloop: meter shutdown: %v\n", err)
			}
		}()
	}

	tracker, err := mergeloop.LoadTracker(repo, "")
	if err != nil {
		return fmt.Errorf("loading tracker: %w", err)
	}

	policy := mergeloop.NewPolicy()
	policy.MaxAgentAttempts = opts.maxAttempts

	deps := mergeloop.Deps{
		Lister:  &ghLister{},
		Rebaser: &safeRebaser{dryRun: opts.dryRun},
		Merger:  &safeMerger{dryRun: opts.dryRun},
		Spawner: &agmSpawner{
			dryRun: opts.dryRun, enabled: opts.enableAgents,
			harness: opts.agentHarness, model: opts.agentModel,
		},
		// Threads clears the GREEN→MERGE blocker: bot review threads (Gemini)
		// left unresolved trip required_conversation_resolution even when CI is
		// green. The driver resolves them just before the merge attempt.
		Threads: &ghThreadResolver{dryRun: opts.dryRun},
		Metrics: mergeloop.NewMetrics(),
	}

	driver := &mergeloop.Driver{
		Repo:           repo,
		Policy:         policy,
		Tracker:        tracker,
		Deps:           deps,
		Cap:            opts.cap,
		StallThreshold: opts.stallThreshold,
		RebaseCooldown: opts.rebaseCooldown,
	}

	switch opts.mode {
	case "tick":
		return doTick(context.Background(), driver, repo)
	case "run":
		return doRun(driver, repo, opts.interval)
	}
	return nil
}

func newMergeLoopFlagSet(mode string, opts *options) *flag.FlagSet {
	fs := flag.NewFlagSet("mergeloop "+mode, flag.ContinueOnError)
	fs.StringVar(&opts.repo, "repo", "", "GitHub repo owner/name (auto-detected if empty)")
	fs.DurationVar(&opts.interval, "interval", 10*time.Minute, "run mode: delay between ticks")
	fs.IntVar(&opts.cap, "cap", 50, "backpressure: skip the tick above this many open PRs")
	fs.IntVar(&opts.maxAttempts, "max-attempts", mergeloop.DefaultMaxAgentAttempts, "max agent fix attempts per PR before escalation")
	fs.DurationVar(&opts.stallThreshold, "stall-threshold", time.Hour, "a PR actionable but untouched longer than this is counted as stalled")
	fs.DurationVar(&opts.rebaseCooldown, "rebase-cooldown", mergeloop.DefaultRebaseCooldown, "minimum delay between two rebases of the same PR; must exceed the slowest required check so CI can finish (0 disables)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "classify and report; perform no rebases/merges/spawns")
	fs.BoolVar(&opts.enableAgents, "enable-agents", false, "spawn AGM agents to fix CI/conflicts; off → defer those PRs")
	fs.StringVar(&opts.agentHarness, "agent-harness", "claude-code", "AGM harness for repair agents")
	fs.StringVar(&opts.agentModel, "agent-model", "", "optional AGM model or model-family alias for repair agents")
	return fs
}

func doTick(ctx context.Context, d *mergeloop.Driver, repo string) error {
	start := time.Now()
	res, err := d.Tick(ctx)
	if err != nil {
		return err
	}
	printSummary(repo, res, time.Since(start))
	return nil
}

func doRun(d *mergeloop.Driver, repo string, interval time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("mergeloop run: %s every %s (Ctrl-C to stop)\n", repo, interval)
	for {
		res, err := d.Tick(ctx)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nmergeloop: shutting down")
				return nil
			}
			fmt.Fprintf(os.Stderr, "mergeloop tick error: %v\n", err)
		} else {
			printSummary(repo, res, 0)
		}
		select {
		case <-ctx.Done():
			fmt.Println("\nmergeloop: shutting down")
			return nil
		case <-time.After(interval):
		}
	}
}

func printSummary(repo string, res mergeloop.TickResult, dur time.Duration) {
	fmt.Printf("mergeloop %s: %d open | merged=%d rebased=%d agents=%d escalated=%d stalled=%d skipped=%d",
		repo, res.OpenPRs, res.Merged, res.Rebased, res.AgentsSpawn, res.Escalated, res.Stalled, res.Skipped)
	if dur > 0 {
		fmt.Printf(" (%s)", dur.Round(time.Millisecond))
	}
	fmt.Println()
}
