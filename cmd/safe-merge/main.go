// Command safe-merge is the authorised, vetted path for merging GitHub PRs.
//
// It enforces AGENTS.md principle 9: an atomic wrapper that cannot be bypassed.
// Raw `gh pr merge` is denied by a PreToolUse hook that redirects here.
//
// Gates (all must pass before merge executes):
//  1. All provider-required CI checks pass — no reds, no pending.
//  2. No unresolved review threads (security-* threads need a written verdict).
//  3. Head commit is ≥ 5 minutes old (soak time).
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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safegit"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-merge: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	// Subcommand dispatch: `safe-merge break-glass <pr>` is the audited,
	// TTY-only escape hatch (docs §4.4/§5 P5). It is deliberately NOT a flag on
	// the normal merge path — break-glass bypasses every gate and must be
	// impossible to trigger accidentally or from an agent.
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
	configPath := fs.String("config", "", "path to .safe-merge.yml (default: repo root; absent → P4 gates skipped)")
	skipReviewCheck := fs.Bool("skip-review-check", false, "bypass unresolved-thread gate (audited; emergency use only)")

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

	// Tracing is opt-in: a no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set
	// (run `otel-local up` and `eval "$(otel-local env)"` to collect spans).
	shutdown := otelsetup.InitTracer("safe-merge")
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "safe-merge: otel shutdown: %v\n", err)
		}
	}()

	return safegit.SafeMergeContext(context.Background(), safegit.MergeConfig{
		PRNumber:        *prNum,
		Repo:            resolvedRepo,
		DryRun:          *dryRun,
		Watch:           *watch,
		WatchTimeout:    *watchTimeout,
		ConfigPath:      *configPath,
		SkipReviewCheck: *skipReviewCheck,
	})
}

const usage = `safe-merge — vetted, gated PR merger (AGENTS.md principle 9 wrapper).

Raw 'gh pr merge' is denied; use this instead.

Usage:
  safe-merge --pr <number> [--repo owner/repo] [flags]

Flags:
  --pr <number>            pull request number to merge (required)
  --repo owner/repo        GitHub repo (default: GITHUB_REPOSITORY env var)
  --watch                  poll until all gates pass (default: one-shot)
  --watch-timeout <dur>    how long to wait in watch mode (default: 45m)
  --dry-run                check gates only; do not execute merge
  --config <path>          path to .safe-merge.yml (default: repo root)
  --skip-review-check      bypass unresolved-thread gate (audited; emergency use only)
  -h, --help               show this help

Gates enforced before merging:
  1. Required CI checks pass (no failures, no pending)
  2. No unresolved review threads
  3. Head commit ≥ 5 minutes old (soak time)
  4. Head is built on the current base tip. A behind branch is advanced to the
     base tip and the attempt blocks: the push invalidates the checks that just
     passed, so the merge waits for the re-run. Runs last, so a CI cycle is
     only spent on a PR that already cleared every other gate. Use --watch to
     advance and merge in one command. A conflicting branch is never rewritten.

P4 gates (only when a .safe-merge.yml is present — see below):
  - expected_reviewers: each listed reviewer must have a review newer than the
    head SHA push. A review that never arrives, or one that predates the last
    push (stale), is surfaced — never a silent pass.
  - flaky_checks: a failing flaky check gets one sanctioned rerun
    (gh run rerun --failed); a second failure is a real block.

.safe-merge.yml (repo root or --config):
  expected_reviewers:
    - login: "some-reviewer[bot]"
      timeout_minutes: 30
      require_newer_than_push: true
  flaky_checks:
    - name: "TestFullLifecycle"
      max_retries: 1
  # Auto-approve decision taxonomy (bead ce-onj5): AUTO-APPROVE by default,
  # HUMAN only for the named high-stakes categories. Shared with the VROOM
  # escalation classifier; omitting it falls back to the built-in default.
  approval_policy:
    auto_approve_default: true
    human_required:
      - name: security-boundary
        reason: "touches a security boundary"
        patterns: ['(?i)\bwrite.?guard\b']

Watch mode:
  With --watch, safe-merge polls every 30 seconds until all gates pass or
  --watch-timeout expires. Useful when CI is still running. Flake-valve retry
  counts persist across watch attempts so a check is never rerun beyond
  max_retries.

Audit log:
  Every attempt is logged to ~/.local/state/dear-agent/safe-merge-audit.jsonl
  (override with SAFE_MERGE_AUDIT_DIR).

Merge execution uses GitHub auto-merge so protected direct-merge policies and
merge queues remain supported. The exact head SHA is pinned.

Post-merge: local worktree and branch are cleaned up automatically.
`

// WatchInterval is re-exported to allow overriding in tests.
var _ = time.Second
