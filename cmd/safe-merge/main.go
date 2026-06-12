// Command safe-merge is the authorised, vetted path for merging GitHub PRs.
//
// It enforces CLAUDE.md principle 9: an atomic wrapper that cannot be bypassed.
// Raw `gh pr merge` is denied by a PreToolUse hook that redirects here.
//
// Gates (all must pass before merge executes):
//  1. ALL CI checks pass — no reds, no pending (required AND non-required).
//  2. No unresolved review threads (security-* threads need a written verdict).
//  3. Head commit is ≥ 5 minutes old (soak time).
//  4. The review bot (gemini-code-assist[bot]) has posted on the current head.
//
// After a successful merge: local worktree and branch are cleaned up.
//
// Subcommands:
//
//	safe-merge --pr <number> [flags]     — normal gated merge
//	safe-merge break-glass --pr <number> — emergency TTY-only bypass (audited)
//
// Examples:
//
//	safe-merge --pr 42
//	safe-merge --pr 42 --repo vbonnet/dear-agent
//	safe-merge --pr 42 --watch
//	safe-merge --pr 42 --dry-run
//	safe-merge break-glass --pr 42
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-merge: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	// Dispatch subcommands before the main flag set.
	if len(argv) > 0 && argv[0] == "break-glass" {
		return runBreakGlass(argv[1:])
	}

	fs := flag.NewFlagSet("safe-merge", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(usage) }

	prNum := fs.Int("pr", 0, "pull request number (required)")
	repo := fs.String("repo", "", "GitHub repo as owner/repo (default: GITHUB_REPOSITORY env var)")
	watch := fs.Bool("watch", false, "poll until all gates pass or timeout elapses")
	watchTimeout := fs.Duration("watch-timeout", safegit.DefaultWatchTimeout, "how long to wait in watch mode")
	dryRun := fs.Bool("dry-run", false, "check gates but do not execute the merge")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	if *prNum == 0 {
		fs.Usage()
		return fmt.Errorf("--pr is required")
	}

	resolvedRepo := resolveRepo(*repo)
	if resolvedRepo == "" {
		return fmt.Errorf("--repo or GITHUB_REPOSITORY must be set")
	}

	return safegit.SafeMerge(safegit.MergeConfig{
		PRNumber:     *prNum,
		Repo:         resolvedRepo,
		DryRun:       *dryRun,
		Watch:        *watch,
		WatchTimeout: *watchTimeout,
	})
}

// runBreakGlass handles the `safe-merge break-glass` subcommand.
func runBreakGlass(argv []string) error {
	fs := flag.NewFlagSet("safe-merge break-glass", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(breakGlassUsage) }

	prNum := fs.Int("pr", 0, "pull request number (required)")
	repo := fs.String("repo", "", "GitHub repo as owner/repo (default: GITHUB_REPOSITORY env var)")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *prNum == 0 {
		fs.Usage()
		return fmt.Errorf("--pr is required")
	}

	resolvedRepo := resolveRepo(*repo)
	if resolvedRepo == "" {
		return fmt.Errorf("--repo or GITHUB_REPOSITORY must be set")
	}

	return safegit.BreakGlass(safegit.BreakGlassConfig{
		PRNumber: *prNum,
		Repo:     resolvedRepo,
	})
}

func resolveRepo(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GITHUB_REPOSITORY")
}

const usage = `safe-merge — vetted, gated PR merger (CLAUDE.md principle 9 wrapper).

Raw 'gh pr merge' is denied; use this instead.

Usage:
  safe-merge --pr <number> [--repo owner/repo] [flags]
  safe-merge break-glass --pr <number>  (emergency, TTY-only, fully audited)

Flags:
  --pr <number>            pull request number to merge (required)
  --repo owner/repo        GitHub repo (default: GITHUB_REPOSITORY env var)
  --watch                  poll until all gates pass (default: one-shot)
  --watch-timeout <dur>    how long to wait in watch mode (default: 45m)
  --dry-run                check gates only; do not execute merge
  -h, --help               show this help

Gates enforced before merging:
  1. ALL CI checks pass (required AND non-required — no failures, no pending)
  2. No unresolved review threads
  3. Head commit ≥ 5 minutes old (soak time)
  4. Expected reviewer(s) have posted on the current head SHA
     (configurable via .safe-merge.yml; default: gemini-code-assist[bot])

Watch mode:
  With --watch, safe-merge polls every 30 seconds until all gates pass or
  --watch-timeout expires. Useful when CI is still running.

Flake valve:
  Checks listed under flaky_checks in .safe-merge.yml get one automatic
  gh run rerun --failed before blocking; a second failure is treated as real.

Audit log:
  Every attempt is logged to ~/.local/state/dear-agent/safe-merge-audit.jsonl
  (override with SAFE_MERGE_AUDIT_DIR).

Post-merge: local worktree and branch are cleaned up automatically.
`

const breakGlassUsage = `safe-merge break-glass — emergency bypass (TTY-only, audited).

Bypasses all safe-merge gates. Requires an interactive terminal — agents are
structurally excluded. Prompts for a written justification (min 20 chars),
posts it as a PR comment, merges, files a P1 bead, and prints a DEAR retro
reminder. Every use triggers a mandatory retro.

Usage:
  safe-merge break-glass --pr <number> [--repo owner/repo]

Flags:
  --pr <number>            pull request number (required)
  --repo owner/repo        GitHub repo (default: GITHUB_REPOSITORY env var)
`

// ensure time import is used for the DefaultWatchTimeout reference.
var _ = time.Second
