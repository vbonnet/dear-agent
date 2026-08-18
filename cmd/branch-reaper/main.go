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
// A lease cannot catch a PR opened in that same window -- opening one does
// not move the tip -- so every delete also re-asks for open head and base
// PRs immediately beforehand and fails closed if it cannot get an answer.
// A delete rejected because the ref is already gone counts as done, not
// failed: that is the end state this asked for.
// The report then names what actually happened -- `deleted` versus
// `delete_failed` -- so a branch still sitting on the remote is never
// listed as reaped. Because PR history is read from GH_REPO while deletes
// push to `origin`, --execute first refuses outright unless origin resolves
// to that same repository.
//
// The protected-branch set is not a hardcoded guess: it is the repo's
// actual default branch (via `gh repo view`) plus every branch filter
// referenced by a push/pull_request `branches:` trigger across
// .github/workflows/*.yml -- a branch CI treats as a long-lived
// integration target must never be auto-deleted, whatever it happens to be
// called -- with `main`/`master` kept as a hardcoded floor in case dynamic
// detection fails entirely. Those entries are matched as filters, not
// literals, so a `release/**` trigger protects the whole family.
//
// A branch whose PR history cannot be retrieved (auth, rate limit,
// transient API error, a per-branch PR count so large the query is
// truncated, or a `gh` call that blows its deadline) is never classified
// from incomplete data -- it is reported in its own `lookup_failed` bucket
// instead, and the run's exit code reflects that a real error occurred. It
// does not abort the rest of the run: this tool walks every branch in the
// repo, so one flaky lookup should not deny a report on everything else.
// Every git/gh subprocess carries its own deadline for the same reason --
// a hung call must cost one branch, not the whole report.
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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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

// prFetchLimit caps `gh pr list` per branch. Classification is only sound
// when the whole PR history for a branch name is in hand — BRR-02 says a
// single open PR anywhere in that history vetoes deletion — so a saturated
// page is treated as an error rather than a partial answer.
const prFetchLimit = 100

// Per-subprocess deadlines. Every git/gh call this tool makes is bounded so
// that a single hung invocation degrades one branch instead of starving the
// whole run until the CI job timeout kills it without a report.
const (
	prFetchTimeout   = 30 * time.Second
	deleteTimeout    = 60 * time.Second
	repoQueryTimeout = 30 * time.Second
)

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

	repo, ok := resolveRepo(ctx, stderr)
	if !ok {
		return exitEnvironment
	}
	if *execute && !originIsRepo(ctx, repo, stderr) {
		return exitEnvironment
	}

	branches, err := listBranches(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "branch-reaper: list branches: %v\n", err)
		return exitEnvironment
	}

	protected, protErr := protectedBranches(ctx, repo)
	if protErr != nil {
		fmt.Fprintf(stderr, "branch-reaper: could not confirm the protected-branch set: %v\n", protErr)
		if *execute {
			fmt.Fprintln(stderr, "branch-reaper: refusing --execute without a confirmed protected-branch set")
			return exitEnvironment
		}
	}

	rep, targets := classifyBranches(ctx, repo, protected, branches, stderr)

	deleteFailed := 0
	if *execute {
		del := func(b branchInfo) error {
			// Classification walked every branch in the repo, which takes
			// long enough for someone to open a PR from or onto this one in
			// the meantime. The SHA lease would not catch that: opening a
			// PR does not move the tip. Re-ask right before the delete, and
			// fail closed if the answer cannot be obtained.
			if err := confirmNoOpenPRs(ctx, repo, b.Name); err != nil {
				return err
			}
			return deleteBranch(ctx, b)
		}
		// Deletion runs before the report is emitted so the report can say
		// what actually happened: `deleted` versus `delete_failed`, never a
		// blanket "auto-deleted" list that includes refs still on the
		// remote.
		deleteFailed = executeSafeDeletes(targets, del, stderr, &rep)
		if deleteFailed > 0 {
			fmt.Fprintf(stderr, "branch-reaper: %d of %d safe deletes failed\n", deleteFailed, len(targets))
		}
	}

	if *jsonOut {
		printJSON(stdout, rep)
	} else {
		printHuman(stdout, rep, time.Now().UTC())
	}

	return exitCode(rep, deleteFailed)
}

// exitCode maps a finished run to its status. A failed deletion outranks
// everything else: it is the only outcome where the tool tried to change
// the remote and did not.
func exitCode(rep Report, deleteFailed int) int {
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

// resolveRepo returns the owner/repo to read PR history from: GH_REPO when
// set, else whatever `gh repo view` infers from the checkout.
func resolveRepo(ctx context.Context, stderr io.Writer) (string, bool) {
	if repo := os.Getenv("GH_REPO"); repo != "" {
		return repo, true
	}
	detected, err := detectRepo(ctx)
	if err != nil || detected == "" {
		fmt.Fprintln(stderr, "branch-reaper: could not determine repo (set GH_REPO or run inside a repo checkout)")
		return "", false
	}
	return detected, true
}

// originIsRepo reports whether the `origin` remote resolves to repo.
//
// PR history is read from repo, but deletion always pushes to origin. If
// GH_REPO names a different repository than the checkout's origin -- a fork
// checkout pointed at upstream, say -- a merged upstream PR could classify
// a same-named fork branch as safe and this tool would delete the wrong
// repository's ref. Refuse rather than delete across that gap. The host is
// part of that identity, so an origin on another forge with a coincidentally
// identical owner/repo path is refused too.
func originIsRepo(ctx context.Context, repo string, stderr io.Writer) bool {
	host, ownerRepo, err := originRemote(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "branch-reaper: --execute needs origin's URL to confirm it is %s: %v\n", repo, err)
		return false
	}
	if !sameRepository(host, ownerRepo, repo) {
		fmt.Fprintf(stderr, "branch-reaper: refusing --execute: PR history is read from %s on %s but origin is %q on %q; "+
			"deletions would target a different repository\n", repo, ghHost(), ownerRepo, host)
		return false
	}
	return true
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
func classifyBranches(ctx context.Context, repo string, protected []string, branches []branchInfo, stderr io.Writer) (Report, []branchInfo) {
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
		// A merged stack branch can still be the BASE of an open child PR.
		// `--head` never sees that child, so without this the base of a
		// live PR would classify as safe_delete and get reaped out from
		// under it. Fork children count here: a fork PR based on this
		// branch is still broken by deleting it.
		basePRs, err := fetchOpenBasePRs(ctx, repo, b.Name)
		if err != nil {
			fmt.Fprintf(stderr, "branch-reaper: base-pr lookup for %q: %v\n", b.Name, err)
			rep.LookupFailed = append(rep.LookupFailed, b.Name)
			continue
		}
		switch classifyBranch(b.TipSHA, prs, basePRs) {
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

// executeSafeDeletes deletes every classified branch from origin, records
// the per-branch outcome in rep.Deleted / rep.DeleteFailed, and returns how
// many deletions failed. Individual failures are logged and do not abort the
// remaining deletes, but they are both counted (the caller turns a non-zero
// count into a non-zero exit) and named in the report, so a branch that is
// still on the remote is never listed as deleted.
func executeSafeDeletes(targets []branchInfo, del func(branchInfo) error, stderr io.Writer, rep *Report) int {
	failed := 0
	for _, b := range targets {
		fmt.Fprintf(stderr, "deleting: %s (%s)\n", b.Name, b.TipSHA)
		if err := del(b); err != nil {
			fmt.Fprintf(stderr, "branch-reaper: delete %s: %v\n", b.Name, err)
			rep.DeleteFailed = append(rep.DeleteFailed, b.Name)
			failed++
			continue
		}
		rep.Deleted = append(rep.Deleted, b.Name)
	}
	return failed
}
