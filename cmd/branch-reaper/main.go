// Command branch-reaper is periodic merged-branch cleanup + abandoned-branch
// visibility.
//
// # Why this exists
//
// GitHub's repo-level "automatically delete head branches" setting is ON for
// this repo and correctly auto-deletes ~98.6% of merged PR branches. But it
// silently fails on a small, unpredictable fraction of merges (no common
// pattern found in merge method, actor, or PR stacking) -- 14 of 1032 merges
// over the repo's history left their branch behind. Left unchecked these
// accumulate forever, because nothing ever goes back and retries the delete.
// Separately, automated branch-creating tooling (spec-coverage generators,
// wayfinder, codex goal branches, review/audit bots) sometimes never opens a
// PR at all, leaving abandoned work branches that neither this tool nor
// GitHub's own mechanism will ever clean up on their own. This tool is the
// safety net for both.
//
// # What it does
//
//  1. SAFE-DELETE (auto, on --execute): a branch whose most-recent-PR is
//     MERGED and whose branch-tip commit is NOT newer than that PR's
//     mergedAt timestamp. That means nothing was ever pushed to the branch
//     after it merged, so the branch's entire content is byte-for-byte what
//     is already in main's history via the squash commit. Zero risk of
//     losing work.
//  2. REVIEW (report only, never deleted): branches with no PR at all, PRs
//     that were closed without merging, or "merged" branches that got new
//     commits pushed after the merge (real unmerged work). These need a
//     human judgment call, so this tool only ever lists them -- it never
//     deletes them.
//
// # Usage
//
//	branch-reaper                # dry run: report only, delete nothing
//	branch-reaper --execute      # actually delete the SAFE-DELETE set
//	branch-reaper --json         # machine-readable report on stdout
//
// Repo is taken from GH_REPO if set, else inferred via `gh repo view`.
//
// REQUIRES: git (run inside a checkout with full history, e.g.
// actions/checkout with fetch-depth: 0), gh (authenticated).
//
// EXIT CODES: 0 = ran clean · 1 = review-list non-empty (visibility signal,
// not a failure) · 3 = usage/environment error.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Bucket names. These are also the JSON report's object keys, so they must
// stay stable — .github/workflows/stale-branch-audit.yml jq-parses them.
const (
	bucketSafeDelete                 = "safe_delete"
	bucketReviewNoPR                 = "review_no_pr"
	bucketReviewClosedUnmerged       = "review_closed_unmerged"
	bucketReviewNewCommitsAfterMerge = "review_new_commits_after_merge"
	// bucketReviewOpenPR is a skip-only classification: a branch with an open
	// PR needs no action and no review, so it is never reported anywhere.
	bucketReviewOpenPR = "review_open_pr"
)

// dependabotPrefix is skipped because dependabot manages its own branch
// lifecycle.
const dependabotPrefix = "dependabot/"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("branch-reaper", flag.ContinueOnError)
	fs.SetOutput(stderr)
	execute := fs.Bool("execute", false, "delete every branch in the safe_delete bucket")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON report on stdout")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: branch-reaper [flags]\n\n"+
			"Reports merged branches GitHub's auto-delete missed, and abandoned\n"+
			"branches that never had a PR, so a human (or --execute) can clean them up.\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 3
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "branch-reaper: unexpected arguments: %v\n", fs.Args())
		return 3
	}

	ctx := context.Background()

	repo := os.Getenv("GH_REPO")
	if repo == "" {
		detected, err := detectRepo(ctx)
		if err != nil || detected == "" {
			fmt.Fprintln(stderr, "branch-reaper: could not determine repo (set GH_REPO or run inside a repo checkout)")
			return 3
		}
		repo = detected
	}

	branches, err := listBranches(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "branch-reaper: list branches: %v\n", err)
		return 3
	}

	rep := classifyBranches(ctx, repo, branches)

	if *jsonOut {
		printJSON(stdout, rep)
	} else {
		printHuman(stdout, rep, time.Now().UTC())
	}

	if *execute {
		executeSafeDeletes(ctx, rep, stderr)
	}

	if reviewTotal(rep) > 0 {
		return 1
	}
	return 0
}

// classifyBranches fetches PR history for each candidate branch and buckets
// it via classifyBranch. Protected and dependabot-managed branches are
// skipped entirely.
func classifyBranches(ctx context.Context, repo string, branches []branchInfo) Report {
	rep := Report{
		SafeDelete:                 []string{},
		ReviewNoPR:                 []string{},
		ReviewClosedUnmerged:       []string{},
		ReviewNewCommitsAfterMerge: []string{},
	}
	for _, b := range branches {
		if isProtected(b.Name) || strings.HasPrefix(b.Name, dependabotPrefix) {
			continue
		}
		prs, err := fetchPRs(ctx, repo, b.Name)
		if err != nil {
			// Matches the shell original: a failed `gh pr list` for one
			// branch degrades to "no PRs found" for that branch rather than
			// aborting the whole run.
			prs = nil
		}
		switch classifyBranch(b.TipDate, prs) {
		case bucketSafeDelete:
			rep.SafeDelete = append(rep.SafeDelete, b.Name)
		case bucketReviewNoPR:
			rep.ReviewNoPR = append(rep.ReviewNoPR, b.Name)
		case bucketReviewClosedUnmerged:
			rep.ReviewClosedUnmerged = append(rep.ReviewClosedUnmerged, b.Name)
		case bucketReviewNewCommitsAfterMerge:
			rep.ReviewNewCommitsAfterMerge = append(rep.ReviewNewCommitsAfterMerge, b.Name)
		case bucketReviewOpenPR:
			// no action, not reported
		}
	}
	return rep
}

// executeSafeDeletes deletes every branch in rep.SafeDelete from origin,
// logging (without aborting) any individual failure.
func executeSafeDeletes(ctx context.Context, rep Report, stderr io.Writer) {
	for _, b := range rep.SafeDelete {
		fmt.Fprintf(stderr, "deleting: %s\n", b)
		if err := deleteBranch(ctx, b); err != nil {
			fmt.Fprintf(stderr, "branch-reaper: delete %s: %v\n", b, err)
		}
	}
}

// ---- report shape ---------------------------------------------------------

// Report is the four-bucket classification of every non-protected remote
// branch. Field names map to the JSON keys stale-branch-audit.yml jq-parses.
type Report struct {
	SafeDelete                 []string `json:"safe_delete"`
	ReviewNoPR                 []string `json:"review_no_pr"`
	ReviewClosedUnmerged       []string `json:"review_closed_unmerged"`
	ReviewNewCommitsAfterMerge []string `json:"review_new_commits_after_merge"`
}

func reviewTotal(r Report) int {
	return len(r.ReviewNoPR) + len(r.ReviewClosedUnmerged) + len(r.ReviewNewCommitsAfterMerge)
}

func printJSON(w io.Writer, r Report) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(r)
}

func printHuman(w io.Writer, r Report, now time.Time) {
	fmt.Fprintf(w, "## Branch reaper — %s\n\n", now.Format("2006-01-02"))
	fmt.Fprintf(w, "Safe to delete (merged, nothing pushed since): %d\n", len(r.SafeDelete))
	for _, b := range r.SafeDelete {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Review: no PR ever opened: %d\n", len(r.ReviewNoPR))
	for _, b := range r.ReviewNoPR {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Review: PR closed without merging: %d\n", len(r.ReviewClosedUnmerged))
	for _, b := range r.ReviewClosedUnmerged {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Review: merged, but new commits pushed after: %d\n", len(r.ReviewNewCommitsAfterMerge))
	for _, b := range r.ReviewNewCommitsAfterMerge {
		fmt.Fprintf(w, "  - %s\n", b)
	}
}

// ---- pure classification (unit-tested) ------------------------------------

// prRecord is the subset of `gh pr list --json number,state,mergedAt,createdAt`
// we reason about.
type prRecord struct {
	Number    int    `json:"number"`
	State     string `json:"state"` // OPEN | MERGED | CLOSED
	MergedAt  string `json:"mergedAt"`
	CreatedAt string `json:"createdAt"`
}

// isProtected reports whether branch is a protected default branch (or the
// remote HEAD symref, which listBranches already filters separately).
func isProtected(branch string) bool {
	switch branch {
	case "main", "master", "HEAD":
		return true
	default:
		return false
	}
}

// classifyBranch buckets one branch given its tip commit's committer date
// (RFC3339 / git iso-strict) and its PR history. It mirrors
// scripts/branch-reaper.sh's decision tree exactly:
//
//  1. any OPEN PR -> review_open_pr (skip, no action, never reported)
//  2. else the most-recently-merged PR (by mergedAt), if any:
//     tip <= mergedAt -> safe_delete (nothing pushed since merge)
//     tip >  mergedAt, or either timestamp is unparseable -> review_new_commits_after_merge
//  3. else any CLOSED PR -> review_closed_unmerged
//  4. else -> review_no_pr
func classifyBranch(tipDateRaw string, prs []prRecord) string {
	for _, pr := range prs {
		if pr.State == "OPEN" {
			return bucketReviewOpenPR
		}
	}

	if merged, ok := lastMerged(prs); ok {
		tip, tipErr := parseTimestamp(tipDateRaw)
		mergedAt, mergedErr := parseTimestamp(merged.MergedAt)
		if tipErr == nil && mergedErr == nil && !tip.After(mergedAt) {
			return bucketSafeDelete
		}
		return bucketReviewNewCommitsAfterMerge
	}

	for _, pr := range prs {
		if pr.State == "CLOSED" {
			return bucketReviewClosedUnmerged
		}
	}
	return bucketReviewNoPR
}

// lastMerged returns the MERGED pr with the latest mergedAt timestamp
// (string-max, matching `jq sort_by(.mergedAt) | last` on RFC3339 strings),
// or ok=false if none of prs is MERGED.
func lastMerged(prs []prRecord) (prRecord, bool) {
	var best prRecord
	found := false
	for _, pr := range prs {
		if pr.State != "MERGED" {
			continue
		}
		if !found || pr.MergedAt > best.MergedAt {
			best = pr
			found = true
		}
	}
	return best, found
}

// parseTimestamp parses an RFC3339 timestamp, which covers both gh's
// mergedAt ("2026-06-10T12:00:00Z") and git's `committerdate:iso-strict`
// ("2026-06-10T12:00:00+00:00") formats.
func parseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// ---- git / gh I/O -----------------------------------------------------

// branchInfo is one remote branch and its tip commit's committer date.
type branchInfo struct {
	Name    string
	TipDate string
}

// listBranches enumerates refs/remotes/origin/* branches (excluding the
// remote HEAD symref) with their tip commit's committer date.
func listBranches(ctx context.Context) ([]branchInfo, error) {
	// #nosec G204,G702 -- fixed "git" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "git", "for-each-ref",
		"--format=%(refname)|%(committerdate:iso-strict)", "refs/remotes/origin")
	out, err := runCombined(cmd)
	if err != nil {
		return nil, err
	}
	return parseBranchList(out), nil
}

// parseBranchList parses `git for-each-ref
// --format=%(refname)|%(committerdate:iso-strict)` output into branchInfo
// entries, dropping the remote HEAD symref and stripping the
// "refs/remotes/origin/" prefix down to a plain branch name.
func parseBranchList(out string) []branchInfo {
	var branches []branchInfo
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}
		refname, tipdate, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		if refname == "refs/remotes/origin/HEAD" {
			continue
		}
		name := strings.TrimPrefix(refname, "refs/remotes/origin/")
		if name == "" {
			continue
		}
		branches = append(branches, branchInfo{Name: name, TipDate: tipdate})
	}
	return branches
}

// deleteBranch deletes branch on origin.
func deleteBranch(ctx context.Context, branch string) error {
	// #nosec G204,G702 -- fixed "git" binary; branch comes from a prior
	// `git for-each-ref` listing of this repo's own remote refs, not
	// external input.
	cmd := exec.CommandContext(ctx, "git", "push", "origin", "--delete", branch)
	_, err := runCombined(cmd)
	return err
}

// fetchPRs returns branch's PR history (all states, most recent 20).
func fetchPRs(ctx context.Context, repo, branch string) ([]prRecord, error) {
	// #nosec G204,G702 -- fixed "gh" binary; repo/branch are argv elements, not
	// shell input.
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo, "--head", branch, "--state", "all",
		"--json", "number,state,mergedAt,createdAt", "--limit", "20")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh pr list: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	raw := bytes.TrimSpace(out.Bytes())
	if len(raw) == 0 {
		return nil, nil
	}
	var prs []prRecord
	if err := json.Unmarshal(raw, &prs); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	return prs, nil
}

// detectRepo infers owner/repo via `gh repo view`.
func detectRepo(ctx context.Context) (string, error) {
	// #nosec G204,G702 -- fixed "gh" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "gh", "repo", "view",
		"--json", "nameWithOwner", "--jq", ".nameWithOwner")
	out, err := runCombined(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// runCombined runs cmd and returns stdout, folding stderr into the error.
func runCombined(cmd *exec.Cmd) (string, error) {
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return "", fmt.Errorf("%s: %w: %s", cmd.Args[0], err, msg)
		}
		return "", fmt.Errorf("%s: %w", cmd.Args[0], err)
	}
	return out.String(), nil
}
