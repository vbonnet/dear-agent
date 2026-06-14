// Command safe-merge is the authorised, vetted path for merging GitHub PRs.
//
// It enforces CLAUDE.md principle 9: an atomic wrapper that cannot be bypassed.
// Raw `gh pr merge` is denied by a PreToolUse hook that redirects here.
//
// Gates (all must pass before merge executes):
//  1. ALL CI checks pass — no reds, no pending (required only).
//  2. No unresolved review threads (security-* threads need a written verdict).
//  3. Head commit is ≥ 5 minutes old (soak time).
//  4. The review bot (gemini-code-assist) has posted.
//
// After a successful merge: local worktree and branch are cleaned up.
//
// Usage:
//
//	safe-merge --pr <number> [--repo owner/repo] [--watch] [--watch-timeout 45m] [--dry-run]
//
// Examples:
//
//	safe-merge --pr 42
//	safe-merge --pr 42 --repo vbonnet/dear-agent
//	safe-merge --pr 42 --watch
//	safe-merge --pr 42 --dry-run
package main

import (
	"errors"
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
	fs := flag.NewFlagSet("safe-merge", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(usage) }

	prNum := fs.Int("pr", 0, "pull request number (required)")
	repo := fs.String("repo", "", "GitHub repo as owner/repo (default: GITHUB_REPOSITORY env var)")
	watch := fs.Bool("watch", false, "poll until all gates pass or timeout elapses")
	watchTimeout := fs.Duration("watch-timeout", safegit.DefaultWatchTimeout, "how long to wait in watch mode")
	dryRun := fs.Bool("dry-run", false, "check gates but do not execute the merge")
	skipBotReview := fs.Bool("skip-bot-review", false, "bypass the Gemini bot-review gate (requires --skip-bot-review-reason)")
	skipBotReviewReason := fs.String("skip-bot-review-reason", "", "required justification when --skip-bot-review is set; recorded in audit log")

	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *prNum == 0 {
		fs.Usage()
		return fmt.Errorf("--pr is required")
	}

	resolvedRepo := *repo
	if resolvedRepo == "" {
		resolvedRepo = os.Getenv("GITHUB_REPOSITORY")
	}
	if resolvedRepo == "" {
		return fmt.Errorf("--repo or GITHUB_REPOSITORY must be set")
	}

	return safegit.SafeMerge(safegit.MergeConfig{
		PRNumber:            *prNum,
		Repo:                resolvedRepo,
		DryRun:              *dryRun,
		Watch:               *watch,
		WatchTimeout:        *watchTimeout,
		SkipBotReview:       *skipBotReview,
		SkipBotReviewReason: *skipBotReviewReason,
	})
}

const usage = `safe-merge — vetted, gated PR merger (CLAUDE.md principle 9 wrapper).

Raw 'gh pr merge' is denied; use this instead.

Usage:
  safe-merge --pr <number> [--repo owner/repo] [flags]

Flags:
  --pr <number>                  pull request number to merge (required)
  --repo owner/repo              GitHub repo (default: GITHUB_REPOSITORY env var)
  --watch                        poll until all gates pass (default: one-shot)
  --watch-timeout <dur>          how long to wait in watch mode (default: 45m)
  --dry-run                      check gates only; do not execute merge
  --skip-bot-review              bypass Gemini bot-review gate (requires --skip-bot-review-reason)
  --skip-bot-review-reason <s>   justification for skipping; recorded in audit log
  -h, --help                     show this help

Gates enforced before merging:
  1. Required CI checks pass (no failures, no pending)
  2. No unresolved review threads
  3. Head commit ≥ 5 minutes old (soak time)
  4. Review bot (gemini-code-assist) has posted
     (skippable with --skip-bot-review --skip-bot-review-reason "..." when bot is unreachable)

Watch mode:
  With --watch, safe-merge polls every 30 seconds until all gates pass or
  --watch-timeout expires. Useful when CI is still running.

Audit log:
  Every attempt is logged to ~/.local/state/dear-agent/safe-merge-audit.jsonl
  (override with SAFE_MERGE_AUDIT_DIR).

Post-merge: local worktree and branch are cleaned up automatically.
`

// WatchInterval is re-exported to allow overriding in tests.
var _ = time.Second
