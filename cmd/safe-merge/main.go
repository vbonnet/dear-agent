// Command safe-merge is the authorised, vetted path for merging GitHub PRs.
//
// It enforces CLAUDE.md principle 9: an atomic wrapper that cannot be bypassed.
// Raw `gh pr merge` is denied by a PreToolUse hook that redirects here.
//
// Gates (all must pass before merge executes):
//  1. All REQUIRED CI checks pass — no reds, no pending.
//  2. No unresolved review threads (security-* threads need a written verdict).
//  3. Head commit is ≥ 5 minutes old (soak time).
//  4. The review bot (gemini-code-assist[bot]) has posted.
//
// After a successful merge: local worktree and branch are cleaned up.
//
// Usage:
//
//	safe-merge --pr <number> [--repo owner/repo]
//
// Examples:
//
//	safe-merge --pr 42
//	safe-merge --pr 42 --repo vbonnet/dear-agent
package main

import (
	"flag"
	"fmt"
	"os"

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

	if err := fs.Parse(argv); err != nil {
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
		PRNumber: *prNum,
		Repo:     resolvedRepo,
	})
}

const usage = `safe-merge — vetted, gated PR merger (CLAUDE.md principle 9 wrapper).

Raw 'gh pr merge' is denied; use this instead.

Usage:
  safe-merge --pr <number> [--repo owner/repo]

Flags:
  --pr <number>      pull request number to merge (required)
  --repo owner/repo  GitHub repo (default: GITHUB_REPOSITORY env var)
  -h, --help         show this help

Gates enforced before merging:
  1. All REQUIRED CI checks pass (no failures, no pending)
  2. No unresolved review threads
  3. Head commit ≥ 5 minutes old (soak time)
  4. Review bot (gemini-code-assist[bot]) has posted

Post-merge: local worktree and branch are cleaned up automatically.
`
