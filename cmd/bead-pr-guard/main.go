// bead-pr-guard checks for an existing open PR that references a bead ID
// before a new PR is created for that bead.
//
// A bead is considered "claimed" if any open PR's title, body, or head branch
// name contains the bead ID. This prevents the duplicate-PR drift that
// surfaced during the 2026-06-12 burndown (ce-6as.80 stamped on 6 open PRs,
// ce-zero-test triplicated as #346/#375/#379).
//
// Usage:
//
//	bead-pr-guard --bead <id> [--repo owner/name]
//
// Exit codes:
//
//	0  No open PR already claims this bead — safe to create.
//	2  At least one open PR already claims this bead — abort.
//	1  Unexpected error (gh CLI unavailable, permission failure, etc.).
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("bead-pr-guard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		beadID = fs.String("bead", "", "bead ID to check (required)")
		repo   = fs.String("repo", "", "GitHub repo in owner/name form (defaults to git remote)")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: bead-pr-guard --bead <id> [--repo owner/name]\n\n"+
			"Checks for an existing open PR that references the given bead ID.\n"+
			"Exit 0 = safe to create; exit 2 = duplicate detected; exit 1 = error.\n\n"+
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
		detected, err := detectRepo()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect GitHub repo: %v\n", err)
			fmt.Fprintf(stderr, "hint: run inside a git repo or pass --repo owner/name\n")
			return 1
		}
		*repo = detected
	}

	claims, err := findClaimingPRs(*repo, *beadID)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if len(claims) == 0 {
		fmt.Fprintf(stdout, "ok: no open PR claims bead %s — safe to create\n", *beadID)
		return 0
	}

	fmt.Fprintf(stderr, "DUPLICATE: bead %s is already claimed by %d open PR(s):\n", *beadID, len(claims))
	for _, c := range claims {
		fmt.Fprintf(stderr, "  #%d — %s [branch: %s]\n", c.Number, c.Title, c.HeadRefName)
	}
	fmt.Fprintf(stderr, "\nTo fix: adopt an existing PR instead of creating a new one.\n")
	fmt.Fprintf(stderr, "    gh pr view %d  # inspect the existing PR\n", claims[0].Number)
	return 2
}
