// bead-close-guard enforces Definition of Done before a bead can be closed.
//
// A bead that references PR(s) may only be closed when ALL referenced PRs
// are merged to main. This prevents the DoD violation where beads are marked
// "closed" while their PRs sit unmerged — work that appears done but isn't.
//
// Usage:
//
//	bead-close-guard --bead <id> [--repo owner/name] [--beads-dir /path] [--force] [--verify-only]
//
// Exit codes:
//
//	0  All referenced PRs are merged (or no PRs referenced) — safe to close.
//	2  At least one referenced PR is not merged — close is blocked.
//	1  Unexpected error (gh unavailable, bead not found, etc.).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bead-close-guard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		beadID     = fs.String("bead", "", "bead ID to check (required)")
		repo       = fs.String("repo", "", "GitHub repo in owner/name form (defaults to git remote)")
		beadsDir   = fs.String("beads-dir", "", "path to the .beads directory (e.g. ~/beads/context-engine/.beads)")
		force      = fs.Bool("force", false, "allow close even with unmerged PRs (for abandoned beads)")
		verifyOnly = fs.Bool("verify-only", false, "check DoD without closing — use as 'bd verify'")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: bead-close-guard --bead <id> [flags]\n\n"+
			"Enforces Definition of Done: blocks bead close when referenced PRs are not merged.\n\n"+
			"Exit 0 = all PRs merged, safe to close\n"+
			"Exit 2 = unmerged PRs found, close blocked\n"+
			"Exit 1 = error\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *beadID == "" {
		fmt.Fprintln(stderr, "error: --bead is required")
		fs.Usage()
		return 1
	}

	if *repo == "" {
		detected, err := detectRepo(*beadsDir)
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect GitHub repo: %v\n", err)
			fmt.Fprintf(stderr, "hint: run inside a git repo or pass --repo owner/name\n")
			return 1
		}
		*repo = detected
	}

	cfg := GuardConfig{
		BeadID:   *beadID,
		Repo:     *repo,
		BeadsDir: *beadsDir,
		Force:    *force,
	}

	result, err := CheckDoD(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if *verifyOnly {
		fmt.Fprintf(stdout, "--- DoD verification for bead %s ---\n", *beadID)
		if result.Title != "" && result.Title != "(unknown)" {
			fmt.Fprintf(stdout, "Title: %s\n", result.Title)
		}
		fmt.Fprintf(stdout, "PRs referenced: %v\n", result.PRs)
		if len(result.UnmergedPR) > 0 {
			for _, pr := range result.UnmergedPR {
				fmt.Fprintf(stdout, "  ✗ PR #%d — %s (not merged)\n", pr.Number, pr.State)
			}
			fmt.Fprintf(stdout, "\nDoD status: FAILING — %d PR(s) not merged\n", len(result.UnmergedPR))
			return 2
		}
		fmt.Fprintf(stdout, "DoD status: PASSING\n")
		return 0
	}

	FormatResult(result, stdout)
	if !result.Passed {
		return 2
	}
	return 0
}
