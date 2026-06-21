package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

// runBreakGlass parses `safe-merge break-glass <pr>` arguments and invokes the
// audited, TTY-only break-glass flow. The PR may be given positionally or via
// --pr; --repo defaults to GITHUB_REPOSITORY.
func runBreakGlass(argv []string) error {
	fs := flag.NewFlagSet("safe-merge break-glass", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(breakGlassUsage) }

	prFlag := fs.String("pr", "", "pull request number or URL (or pass positionally)")
	repo := fs.String("repo", "", "GitHub repo as owner/repo (default: GITHUB_REPOSITORY env var)")

	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	prArg := *prFlag
	if prArg == "" && fs.NArg() > 0 {
		prArg = fs.Arg(0)
	}
	if prArg == "" {
		fs.Usage()
		return fmt.Errorf("break-glass requires a PR number or URL")
	}

	resolvedRepo := *repo
	if resolvedRepo == "" {
		resolvedRepo = os.Getenv("GITHUB_REPOSITORY")
	}
	if resolvedRepo == "" {
		return fmt.Errorf("--repo or GITHUB_REPOSITORY must be set")
	}

	return safegit.BreakGlass(safegit.BreakGlassConfig{
		PRArg: prArg,
		Repo:  resolvedRepo,
	})
}

const breakGlassUsage = `safe-merge break-glass — audited, TTY-only merge escape hatch.

Bypasses ALL safe-merge gates. For HUMANS ONLY: requires an interactive TTY,
so agent callers are structurally excluded. Every use posts the reason to the
PR, appends an append-only audit record, and files a P1 retro bead.

Usage:
  safe-merge break-glass <pr> [--repo owner/repo]
  safe-merge break-glass --pr <pr> [--repo owner/repo]

Flags:
  --pr <number|url>   pull request to merge (or pass positionally)
  --repo owner/repo   GitHub repo (default: GITHUB_REPOSITORY env var)
  -h, --help          show this help

Behaviour:
  1. Refuses to run without an interactive TTY.
  2. Prompts for a typed reason (min 20 chars).
  3. Posts the reason as a PR comment (reason + operator + timestamp).
  4. Appends an audit record to ~/.config/dear-agent/break-glass-audit.jsonl.
  5. Files a P1 retro bead for post-incident review.
  6. On ruleset-protected repos: prints the Settings-edit instruction (no merge).
  7. Otherwise: squash-merges immediately and prints a DEAR retro notice.
`
