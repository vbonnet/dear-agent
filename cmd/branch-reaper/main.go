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
//  1. SAFE-DELETE (auto, on --execute): a branch with no open PR whose
//     most-recently-merged PR's recorded head commit is byte-for-byte the
//     branch's current tip. Identity of the SHA -- not a timestamp
//     comparison -- is what proves the branch holds no commit that is not
//     already in main's history via the squash commit. A timestamp cannot
//     prove that: a force-push after the merge can land an older commit
//     whose committer date predates mergedAt, which would read as "nothing
//     pushed since" while actually discarding unmerged work.
//  2. REVIEW (report only, never deleted): branches with no PR at all, PRs
//     that were closed without merging, or "merged" branches whose tip no
//     longer matches the merged PR head (real unmerged work). These need a
//     human judgment call, so this tool only ever lists them -- it never
//     deletes them.
//
// Deletion itself is leased: every delete carries
// --force-with-lease=<branch>:<sha> pinned to the exact SHA that was
// classified, so a push that lands between classification and deletion
// makes the delete fail loudly instead of silently destroying the new tip.
//
// The protected-branch set is not a hardcoded guess: it is the repo's
// actual default branch (via `gh repo view`) plus every branch referenced
// by a push/pull_request `branches:` trigger across .github/workflows/*.yml
// -- a branch CI treats as a long-lived integration target must never be
// auto-deleted, whatever it happens to be called -- with `main`/`master`
// kept as a hardcoded floor in case dynamic detection fails entirely.
//
// A branch whose PR history cannot be retrieved (auth, rate limit,
// transient API error, or a per-branch PR count so large the query is
// truncated) is never classified from incomplete data -- it is reported in
// its own `lookup_failed` bucket instead, and the run's exit code reflects
// that a real error occurred. It does not abort the rest of the run: this
// tool walks every branch in the repo, so one flaky lookup should not deny
// a report on everything else.
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
// not a failure) · 2 = one or more branches' PR history could not be
// retrieved (operational error, those branches were skipped) · 3 =
// usage/environment error · 4 = at least one safe-delete could not be
// deleted.
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
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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

// prFetchLimit caps `gh pr list` per branch. Classification is only sound
// when the whole PR history for a branch name is in hand — BRR-02 says a
// single open PR anywhere in that history vetoes deletion — so a saturated
// page is treated as an error rather than a partial answer.
const prFetchLimit = 100

// Exit codes. See the package doc comment; stale-branch-audit.yml treats
// exitEnvironment as a hard failure and everything else (0/1/2/4) as "emit
// the report, then decide whether to open a review issue".
const (
	exitClean        = 0
	exitReview       = 1
	exitLookupFailed = 2
	exitEnvironment  = 3
	exitDeleteError  = 4
)

// workflowsDir is where GitHub Actions workflow files live, relative to the
// repo root this tool is expected to run from (same assumption listBranches
// and detectRepo already make: run inside a full checkout at repo root).
const workflowsDir = ".github/workflows"

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
			return exitClean
		}
		return exitEnvironment
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "branch-reaper: unexpected arguments: %v\n", fs.Args())
		return exitEnvironment
	}

	ctx := context.Background()

	repo := os.Getenv("GH_REPO")
	if repo == "" {
		detected, err := detectRepo(ctx)
		if err != nil || detected == "" {
			fmt.Fprintln(stderr, "branch-reaper: could not determine repo (set GH_REPO or run inside a repo checkout)")
			return exitEnvironment
		}
		repo = detected
	}

	branches, err := listBranches(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "branch-reaper: list branches: %v\n", err)
		return exitEnvironment
	}

	protected := protectedBranches(ctx, repo)

	rep, targets := classifyBranches(ctx, repo, protected, branches, stderr)

	if *jsonOut {
		printJSON(stdout, rep)
	} else {
		printHuman(stdout, rep, time.Now().UTC())
	}

	deleteFailed := 0
	if *execute {
		del := func(b branchInfo) error { return deleteBranch(ctx, b) }
		deleteFailed = executeSafeDeletes(targets, del, stderr)
		if deleteFailed > 0 {
			fmt.Fprintf(stderr, "branch-reaper: %d of %d safe deletes failed\n", deleteFailed, len(targets))
		}
	}

	switch {
	case deleteFailed > 0:
		return exitDeleteError
	case len(rep.LookupFailed) > 0:
		return exitLookupFailed
	case reviewTotal(rep) > 0:
		return exitReview
	default:
		return exitClean
	}
}

// classifyBranches fetches PR history for each candidate branch and buckets
// it via classifyBranch. Protected and dependabot-managed branches are
// skipped entirely. It returns the report plus the safe-delete branches with
// the exact tip SHA each was classified against, which executeSafeDeletes
// leases its deletes on.
//
// A failed PR lookup (auth, rate limit, transient API error, or a PR count
// so large the query is truncated) never degrades to "no PRs found" --
// that would silently reclassify a possibly-merged branch as never-had-a-PR,
// which is a materially wrong claim for a tool whose whole job is deciding
// what is safe to delete. Instead the branch is reported in
// Report.LookupFailed and left unclassified; it is never a candidate for
// deletion. This does not abort the run for the remaining branches -- a
// single flaky lookup should not deny a report on everything else.
func classifyBranches(ctx context.Context, repo string, protected map[string]bool, branches []branchInfo, stderr io.Writer) (Report, []branchInfo) {
	rep := Report{
		SafeDelete:                 []string{},
		ReviewNoPR:                 []string{},
		ReviewClosedUnmerged:       []string{},
		ReviewNewCommitsAfterMerge: []string{},
		LookupFailed:               []string{},
	}
	var targets []branchInfo
	for _, b := range branches {
		if isProtected(b.Name, protected) || strings.HasPrefix(b.Name, dependabotPrefix) {
			continue
		}
		prs, err := fetchPRs(ctx, repo, b.Name)
		if err != nil {
			fmt.Fprintf(stderr, "branch-reaper: pr lookup for %q: %v\n", b.Name, err)
			rep.LookupFailed = append(rep.LookupFailed, b.Name)
			continue
		}
		switch classifyBranch(b.TipSHA, prs) {
		case bucketSafeDelete:
			rep.SafeDelete = append(rep.SafeDelete, b.Name)
			targets = append(targets, b)
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
	return rep, targets
}

// executeSafeDeletes deletes every classified branch from origin and returns
// how many deletions failed. Individual failures are logged and do not abort
// the remaining deletes, but the count is what the caller turns into a
// non-zero exit so a rejected delete can never be reported as done.
func executeSafeDeletes(targets []branchInfo, del func(branchInfo) error, stderr io.Writer) int {
	failed := 0
	for _, b := range targets {
		fmt.Fprintf(stderr, "deleting: %s (%s)\n", b.Name, b.TipSHA)
		if err := del(b); err != nil {
			fmt.Fprintf(stderr, "branch-reaper: delete %s: %v\n", b.Name, err)
			failed++
		}
	}
	return failed
}

// ---- report shape ---------------------------------------------------------

// Report is the classification of every non-protected remote branch, plus
// the lookup_failed operational-failure bucket. Field names map to the JSON
// keys stale-branch-audit.yml jq-parses.
type Report struct {
	SafeDelete                 []string `json:"safe_delete"`
	ReviewNoPR                 []string `json:"review_no_pr"`
	ReviewClosedUnmerged       []string `json:"review_closed_unmerged"`
	ReviewNewCommitsAfterMerge []string `json:"review_new_commits_after_merge"`
	// LookupFailed lists branches whose PR-history lookup itself failed
	// (auth, rate limit, transient API error, or truncation past the
	// per-branch fetch limit) -- never classified, never deleted, distinct
	// from review_no_pr.
	LookupFailed []string `json:"lookup_failed"`
}

func reviewTotal(r Report) int {
	return len(r.ReviewNoPR) + len(r.ReviewClosedUnmerged) + len(r.ReviewNewCommitsAfterMerge)
}

func printJSON(w io.Writer, r Report) {
	if r.LookupFailed == nil {
		r.LookupFailed = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(r)
}

func printHuman(w io.Writer, r Report, now time.Time) {
	fmt.Fprintf(w, "## Branch reaper — %s\n\n", now.Format("2006-01-02"))
	fmt.Fprintf(w, "Safe to delete (merged, tip still the merged head): %d\n", len(r.SafeDelete))
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
	fmt.Fprintf(w, "Review: merged, but tip moved off the merged head: %d\n", len(r.ReviewNewCommitsAfterMerge))
	for _, b := range r.ReviewNewCommitsAfterMerge {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Lookup failed (gh error, not classified): %d\n", len(r.LookupFailed))
	for _, b := range r.LookupFailed {
		fmt.Fprintf(w, "  - %s\n", b)
	}
}

// ---- pure classification (unit-tested) ------------------------------------

// prRecord is the subset of `gh pr list --json ...` we reason about.
type prRecord struct {
	Number   int    `json:"number"`
	State    string `json:"state"` // OPEN | MERGED | CLOSED
	MergedAt string `json:"mergedAt"`
	// HeadRefOid is the commit GitHub recorded as this PR's head. For a
	// MERGED PR that is exactly the content the squash commit carried into
	// main, which is what makes SHA equality a proof of redundancy.
	HeadRefOid string `json:"headRefOid"`
	// IsCrossRepository marks a PR opened from a fork. Such a PR says
	// nothing about the same-named branch in this repository, so it is
	// filtered out before classification.
	IsCrossRepository bool `json:"isCrossRepository"`
}

// isProtected reports whether branch must never be classified or deleted,
// regardless of PR state.
func isProtected(branch string, protected map[string]bool) bool {
	return protected[branch]
}

// baseProtectedBranches is the hardcoded floor that always applies,
// regardless of whether the dynamic detection in protectedBranches
// succeeds: these names (plus the remote HEAD symref, which listBranches
// already filters out before this is ever consulted) are never eligible
// for deletion.
func baseProtectedBranches() map[string]bool {
	return map[string]bool{"main": true, "master": true, "HEAD": true}
}

// protectedBranches returns baseProtectedBranches() plus the repository's
// actual default branch (via `gh repo view`) and every branch name
// referenced by a push/pull_request `branches:` trigger across
// .github/workflows/*.yml. A branch CI treats as a long-lived integration
// target (this repo's `develop`, for instance -- see the push trigger in
// .github/workflows/ci.yml) must never be auto-deleted even if some PR
// happened to merge into it, so this derives the protected set from what
// the repo's own workflows actually do rather than hardcoding an assumed
// list that silently goes stale the day a new long-lived branch shows up.
// Detection failures are non-fatal: they just mean fewer branches get the
// extra protection, never fewer than the hardcoded floor.
func protectedBranches(ctx context.Context, repo string) map[string]bool {
	protected := baseProtectedBranches()
	if def, err := defaultBranch(ctx, repo); err == nil && def != "" {
		protected[def] = true
	}
	for _, b := range workflowTriggerBranches(workflowsDir) {
		protected[b] = true
	}
	return protected
}

// extractTriggerBranches parses a GitHub Actions workflow YAML document and
// returns every branch name listed in a top-level `on.push.branches` or
// `on.pull_request.branches` trigger. Best-effort: any parse or shape
// mismatch just yields no branches for that file, since this is a defensive
// addition to the protected set on top of baseProtectedBranches, not the
// only source of protection.
func extractTriggerBranches(data []byte) []string {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	onRaw, ok := doc["on"]
	if !ok {
		return nil
	}
	onMap, ok := onRaw.(map[string]any)
	if !ok {
		return nil
	}
	var branches []string
	for _, event := range []string{"push", "pull_request"} {
		triggerRaw, ok := onMap[event]
		if !ok {
			continue
		}
		triggerMap, ok := triggerRaw.(map[string]any)
		if !ok {
			continue
		}
		listRaw, ok := triggerMap["branches"]
		if !ok {
			continue
		}
		list, ok := listRaw.([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			if s, ok := item.(string); ok {
				branches = append(branches, s)
			}
		}
	}
	return branches
}

// workflowTriggerBranches reads every *.yml/*.yaml file directly under dir
// and returns the union of extractTriggerBranches across all of them. A
// missing or unreadable dir/file yields no branches rather than an error,
// matching extractTriggerBranches's best-effort contract.
func workflowTriggerBranches(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var branches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- fixed workflows directory, not user input
		if err != nil {
			continue
		}
		branches = append(branches, extractTriggerBranches(data)...)
	}
	return branches
}

// classifyBranch buckets one branch given its tip commit SHA and its PR
// history:
//
//  1. any OPEN PR -> review_open_pr (skip, no action, never reported)
//  2. else the most-recently-merged PR (by mergedAt), if any:
//     its headRefOid == tip SHA -> safe_delete (branch content is already
//     in main via the squash commit)
//     anything else (tip moved, or no head SHA recorded) ->
//     review_new_commits_after_merge
//  3. else any CLOSED PR -> review_closed_unmerged
//  4. else -> review_no_pr
func classifyBranch(tipSHA string, prs []prRecord) string {
	for _, pr := range prs {
		if pr.State == "OPEN" {
			return bucketReviewOpenPR
		}
	}

	if merged, ok := lastMerged(prs); ok {
		if tipSHA != "" && merged.HeadRefOid != "" && strings.EqualFold(merged.HeadRefOid, tipSHA) {
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

// ---- git / gh I/O -----------------------------------------------------

// branchInfo is one remote branch and its tip commit SHA.
type branchInfo struct {
	Name   string
	TipSHA string
}

// branchFieldSep is NUL because it is the one byte a git ref name can never
// contain (git check-ref-format accepts `|`, so a pipe-delimited record is
// ambiguous for a branch literally named `foo|bar`).
const branchFieldSep = "\x00"

// listBranches enumerates refs/remotes/origin/* branches (excluding the
// remote HEAD symref) with their tip commit SHA.
func listBranches(ctx context.Context) ([]branchInfo, error) {
	// #nosec G204,G702 -- fixed "git" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "git", "for-each-ref",
		"--format=%(refname)%00%(objectname)", "refs/remotes/origin")
	out, err := runCombined(cmd)
	if err != nil {
		return nil, err
	}
	return parseBranchList(out), nil
}

// parseBranchList parses `git for-each-ref
// --format=%(refname)%00%(objectname)` output into branchInfo entries,
// dropping the remote HEAD symref and stripping the
// "refs/remotes/origin/" prefix down to a plain branch name.
func parseBranchList(out string) []branchInfo {
	var branches []branchInfo
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}
		refname, sha, ok := strings.Cut(line, branchFieldSep)
		if !ok || sha == "" {
			continue
		}
		if refname == "refs/remotes/origin/HEAD" {
			continue
		}
		name := strings.TrimPrefix(refname, "refs/remotes/origin/")
		if name == "" || name == refname {
			continue
		}
		branches = append(branches, branchInfo{Name: name, TipSHA: sha})
	}
	return branches
}

// deleteBranch deletes b from origin, leased to the exact tip SHA that was
// classified. If anything landed on the branch since, git rejects the push
// with "stale info" rather than destroying the unseen commits.
func deleteBranch(ctx context.Context, b branchInfo) error {
	// #nosec G204,G702 -- fixed "git" binary; branch comes from a prior
	// `git for-each-ref` listing of this repo's own remote refs, not
	// external input.
	cmd := exec.CommandContext(ctx, "git", "push", "origin",
		"--force-with-lease="+b.Name+":"+b.TipSHA, "--delete", b.Name)
	_, err := runCombined(cmd)
	return err
}

// fetchPRs returns branch's PR history for PRs opened from this repository.
// Fork PRs are dropped: `gh pr list --head` filters on head branch *name*
// only, so a fork's same-named branch would otherwise be read as this
// branch's history.
func fetchPRs(ctx context.Context, repo, branch string) ([]prRecord, error) {
	// #nosec G204,G702 -- fixed "gh" binary; repo/branch are argv elements, not
	// shell input.
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo, "--head", branch, "--state", "all",
		"--json", "number,state,mergedAt,headRefOid,isCrossRepository",
		"--limit", fmt.Sprint(prFetchLimit))
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
	if len(prs) >= prFetchLimit {
		return nil, fmt.Errorf("more than %d pull requests reference this branch; "+
			"classification would be based on a truncated history", prFetchLimit)
	}
	return sameRepoPRs(prs), nil
}

// sameRepoPRs drops fork-originated pull requests.
func sameRepoPRs(prs []prRecord) []prRecord {
	kept := make([]prRecord, 0, len(prs))
	for _, pr := range prs {
		if pr.IsCrossRepository {
			continue
		}
		kept = append(kept, pr)
	}
	return kept
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

// defaultBranch returns repo's default branch name via `gh repo view`.
func defaultBranch(ctx context.Context, repo string) (string, error) {
	// #nosec G204,G702 -- fixed "gh" binary; repo is an argv element, not
	// shell input.
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--repo", repo,
		"--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
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
