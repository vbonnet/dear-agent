// Command pr-blockers is the deterministic PR merge-blocker classifier.
//
// Given a PR number it answers, from GitHub's own merge state, exactly what
// blocks the merge and exactly how to fix it. It exists so no agent ever
// guesses at a merge blocker again: the two recurring real blockers
// (unresolved OUTDATED review threads, and a branch out of date with base)
// are invisible to casual inspection but trivially knowable from
// `gh pr view --json mergeStateStatus,...` plus the reviewThreads GraphQL
// with isOutdated. See the DEAR retro
// engram-research/retrospectives/2026-08-18-pr-merge-blocker-guessing.md.
//
// Usage:
//
//	pr-blockers <number> [--repo owner/repo] [--json]
//	pr-blockers --pr <number> [--repo owner/repo] [--json]
//
// Exit codes: 0 = READY or already MERGED; 1 = BLOCKED or CLOSED; 2 = error.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(argv []string, stdout *os.File) int {
	fs := flag.NewFlagSet("pr-blockers", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	prFlag := fs.Int("pr", 0, "pull request number")
	repoFlag := fs.String("repo", "", "GitHub repo as owner/repo (default: GITHUB_REPOSITORY, then the cwd's repo)")
	jsonOut := fs.Bool("json", false, "emit the diagnosis as JSON")

	// Accept a positional PR number before or instead of --pr.
	var positional []string
	var flags []string
	for _, a := range argv {
		if _, err := strconv.Atoi(a); err == nil {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
	}
	if err := fs.Parse(flags); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	prNum := *prFlag
	if prNum == 0 && len(positional) == 1 {
		prNum, _ = strconv.Atoi(positional[0])
	}
	if prNum <= 0 || len(positional) > 1 {
		fs.Usage()
		return 2
	}

	ctx := context.Background()
	repo, err := resolveRepo(ctx, *repoFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pr-blockers: %v\n", err)
		return 2
	}

	d, err := safegit.Diagnose(ctx, prNum, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pr-blockers: %v\n", err)
		return 2
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(d); err != nil {
			fmt.Fprintf(os.Stderr, "pr-blockers: encoding diagnosis: %v\n", err)
			return 2
		}
	} else {
		printHuman(stdout, d)
	}

	switch d.Verdict {
	case safegit.VerdictReady, safegit.VerdictMerged:
		return 0
	default:
		return 1
	}
}

func resolveRepo(ctx context.Context, flagVal string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if env := os.Getenv("GITHUB_REPOSITORY"); env != "" {
		return env, nil
	}
	out, err := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner").Output()
	if err == nil {
		if repo := strings.TrimSpace(string(out)); strings.Contains(repo, "/") {
			return repo, nil
		}
	}
	return "", fmt.Errorf("--repo or GITHUB_REPOSITORY must be set (and the cwd is not a GitHub repo)")
}

func printHuman(w *os.File, d safegit.Diagnosis) {
	fmt.Fprintf(w, "PR #%d in %s: %s\n", d.PR.Number, d.Repo, d.PR.Title)
	fmt.Fprintf(w, "  state=%s draft=%t mergeable=%s mergeStateStatus=%s reviewDecision=%s\n",
		d.PR.State, d.PR.IsDraft, d.PR.Mergeable, d.PR.MergeStateStatus, orNone(d.PR.ReviewDecision))

	switch d.Verdict {
	case safegit.VerdictMerged:
		fmt.Fprintln(w, "\nVerdict: MERGED. Nothing to do.")
		return
	case safegit.VerdictClosed:
		fmt.Fprintln(w, "\nVerdict: CLOSED. Reopening is a human decision; ask the operator.")
		return
	case safegit.VerdictReady:
		fmt.Fprintf(w, "\nVerdict: READY. No blockers. Merge with: safe-merge --pr %d --repo %s\n", d.PR.Number, d.Repo)
		return
	}

	fmt.Fprintf(w, "\nBLOCKERS (%d), in remediation order:\n", len(d.Blockers))
	for i, b := range d.Blockers {
		fmt.Fprintf(w, " %d. %s: %s\n    fix: %s\n", i+1, b.Code, b.Detail, b.Fix)
	}
	fmt.Fprintln(w, "\nVerdict: BLOCKED. This list is exhaustive over GitHub's merge state;")
	fmt.Fprintln(w, "fix the blockers above in order and re-run. Do not investigate anything else.")
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

const usage = `pr-blockers: deterministic PR merge-blocker classifier.

Answers "why won't this PR merge?" from GitHub's own merge state. Run it
BEFORE investigating any stuck PR; never infer a merge blocker from code.

Usage:
  pr-blockers <number> [--repo owner/repo] [--json]

Flags:
  --pr <number>       pull request number (alternative to the positional form)
  --repo owner/repo   GitHub repo (default: GITHUB_REPOSITORY, then cwd's repo)
  --json              emit the full diagnosis as JSON
  -h, --help          show this help

Exit codes:
  0  READY (merge with safe-merge) or already MERGED
  1  BLOCKED (blockers listed with exact fixes) or CLOSED
  2  query/usage error

Blockers detected, with their fixes:
  DRAFT                   gh pr ready <n>   (human flips security/product/money PRs)
  CONFLICTS               safe-rebase onto base, resolve, safe-push
  FAILING_REQUIRED_CHECK  fix the named check (gh pr checks <n>)
  PENDING_REQUIRED_CHECK  gh pr checks <n> --watch
  UNRESOLVED_THREADS      address, then resolve-review-threads resolve-all (outdated threads count!)
  CHANGES_REQUESTED       address the review, push, re-request
  REVIEW_REQUIRED         obtain an approving review
  BEHIND                  gh pr update-branch <n>
`
