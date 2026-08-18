// bead-close-guard enforces Definition of Done before a bead can be closed.
//
// It runs two gates, each strictly stronger than the last
// (see cmd/drift-check/README.md):
//
//   - merged: every PR the bead references must be merged to main. This
//     prevents the DoD violation where beads are marked "closed" while their
//     PRs sit unmerged — work that appears done but isn't.
//   - deployed: when a merged PR changed the source of a deploy target (a
//     Claude Code hook, a launchd plist), that artifact must be current on the
//     host before the bead may close. This closes the "merged != deployed" gap
//     where a fix reaches main but is never redeployed and the machine stays
//     broken (PR #456: a stop-hook fix merged but never installed, leaking
//     processes for two days).
//
// The deployed gate is best-effort on its own infrastructure: when no
// dear-agent checkout / deploy-target config is reachable it is skipped (and
// says so) rather than blocking a legitimate close. A genuine drift always
// blocks.
//
// Usage:
//
//	bead-close-guard --bead <id> [--repo owner/name] [--beads-dir /path]
//	                 [--repo-root /path] [--git-ref <ref>]
//	                 [--abandon-reason <why>] [--verify-only]
//
// Exit codes:
//
//	0  Safe to close: all PRs merged and any touched deploy targets are current.
//	2  Blocked: a referenced PR is not merged, or a merged change is not
//	   deployed on the host.
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
		beadID        = fs.String("bead", "", "bead ID to check (required)")
		repo          = fs.String("repo", "", "GitHub repo in owner/name form (defaults to git remote)")
		beadsDir      = fs.String("beads-dir", "", "path to the .beads directory (e.g. ~/beads/context-engine/.beads)")
		repoRoot      = fs.String("repo-root", "", "dear-agent checkout the deployed gate reads sources from (default: git toplevel of cwd)")
		gitRef        = fs.String("git-ref", "", "deployed gate compares the host against this committed ref instead of the working tree (e.g. origin/main)")
		abandonReason = fs.String("abandon-reason", "", "reason for abandoning this bead despite unmerged PRs or undeployed artifacts (audited)")
		verifyOnly    = fs.Bool("verify-only", false, "check DoD without closing — use as 'bd verify'")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: bead-close-guard --bead <id> [flags]\n\n"+
			"Enforces Definition of Done: blocks bead close when a referenced PR is\n"+
			"not merged, or when a merged change is not deployed on the host.\n\n"+
			"Exit 0 = merged and deployed, safe to close\n"+
			"Exit 2 = unmerged PR or undeployed artifact, close blocked\n"+
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

	root := *repoRoot
	if root == "" {
		// Best-effort: the git toplevel of cwd is the dear-agent checkout when the
		// close runs from there. An empty root simply skips the deployed gate.
		root = detectRepoRoot()
	}

	cfg := GuardConfig{
		BeadID:        *beadID,
		Repo:          *repo,
		BeadsDir:      *beadsDir,
		AbandonReason: *abandonReason,
		RepoRoot:      root,
		GitRef:        *gitRef,
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
		failing := false
		for _, pr := range result.UnmergedPR {
			fmt.Fprintf(stdout, "  ✗ PR #%d — %s (not merged)\n", pr.Number, pr.State)
			failing = true
		}
		if result.DeployTargetsChecked > 0 {
			fmt.Fprintf(stdout, "Deploy targets touched: %d\n", result.DeployTargetsChecked)
		}
		for _, f := range result.DriftFindings {
			fmt.Fprintf(stdout, "  ✗ %s — %s on host (fix: %s)\n", f.Target, f.Status, f.Remediation)
			failing = true
		}
		if result.DeploySkipNote != "" {
			fmt.Fprintf(stdout, "  (deployed gate skipped: %s)\n", result.DeploySkipNote)
		}
		if failing {
			fmt.Fprintf(stdout, "\nDoD status: FAILING\n")
			return 2
		}
		fmt.Fprintf(stdout, "DoD status: PASSING\n")
		return 0
	}

	FormatResult(result, stdout)
	if result.AbandonReason != "" {
		home, _ := os.UserHomeDir()
		var unmergedNums []int
		for _, pr := range result.UnmergedPR {
			unmergedNums = append(unmergedNums, pr.Number)
		}
		if auditErr := AppendAbandonAudit(home, *beadID, result.AbandonReason, unmergedNums); auditErr != nil {
			fmt.Fprintf(stderr, "warning: abandon audit log failed: %v\n", auditErr)
		}
	}
	if !result.Passed {
		// Record the prevented violation in the VROOM decision trail so
		// supervisor ticks and retros can count how often the guard fires.
		// Best-effort: a trail failure never downgrades the block.
		home, _ := os.UserHomeDir()
		var unmergedNums []int
		for _, pr := range result.UnmergedPR {
			unmergedNums = append(unmergedNums, pr.Number)
		}
		if trailErr := AppendDoDViolationTrail(home, *beadID, result.Title, unmergedNums); trailErr != nil {
			fmt.Fprintf(stderr, "warning: vroom trail log failed: %v\n", trailErr)
		}
		return 2
	}
	return 0
}
