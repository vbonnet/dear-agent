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
	"regexp"
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
	// Deleted and DeleteFailed split safe_delete by what --execute actually
	// achieved. Without --execute both are empty: safe_delete alone means
	// "would delete", never "did delete".
	Deleted      []string `json:"deleted"`
	DeleteFailed []string `json:"delete_failed"`
}

func reviewTotal(r Report) int {
	return len(r.ReviewNoPR) + len(r.ReviewClosedUnmerged) + len(r.ReviewNewCommitsAfterMerge)
}

func printJSON(w io.Writer, r Report) {
	if r.LookupFailed == nil {
		r.LookupFailed = []string{}
	}
	if r.Deleted == nil {
		r.Deleted = []string{}
	}
	if r.DeleteFailed == nil {
		r.DeleteFailed = []string{}
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
	if len(r.Deleted) == 0 && len(r.DeleteFailed) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Deleted this run: %d\n", len(r.Deleted))
	for _, b := range r.Deleted {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Deletion FAILED (still on the remote): %d\n", len(r.DeleteFailed))
	for _, b := range r.DeleteFailed {
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
// regardless of PR state. Entries are GitHub Actions branch filters, so an
// entry like `release/**` protects the whole family, not just a branch
// literally named "release/**".
func isProtected(branch string, protected []string) bool {
	for _, pattern := range protected {
		if matchBranchFilter(pattern, branch) {
			return true
		}
	}
	return false
}

// branchFilterMeta is the full set of GitHub Actions filter-pattern
// metacharacters. A pattern containing none of them is a plain branch name.
const branchFilterMeta = `*?+[]\`

// matchBranchFilter reports whether name matches a GitHub Actions branch
// filter, implementing GitHub's filter grammar: `*` matches a run of
// characters within one path segment, `**` matches across `/`, `?` matches
// one character within a segment, `+` matches one or more of the preceding
// character, `[]` is a character class, and `\` escapes the next character.
//
// Negated filters (`!foo`) are exclusions in a workflow trigger, so they
// assert nothing about what CI protects and never protect anything here.
//
// A pattern that cannot be translated fails CLOSED -- it protects the
// branch. Under-protecting means deleting a branch CI treats as long-lived;
// over-protecting only means one fewer branch is reaped this week.
func matchBranchFilter(pattern, name string) bool {
	if pattern == "" || strings.HasPrefix(pattern, "!") {
		return false
	}
	if !strings.ContainsAny(pattern, branchFilterMeta) {
		return pattern == name
	}
	expr, ok := branchFilterRegexp(pattern)
	if !ok {
		return true
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return true
	}
	return re.MatchString(name)
}

// branchFilterRegexp translates a GitHub branch filter into an anchored
// regular expression. ok is false when the pattern uses a construct this
// translator cannot represent faithfully, which callers must treat as
// "assume it matches" rather than "assume it does not".
func branchFilterRegexp(pattern string) (string, bool) {
	var b strings.Builder
	b.WriteString(`\A`)
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(`.*`)
				i++
				continue
			}
			b.WriteString(`[^/]*`)
		case '?':
			b.WriteString(`[^/]`)
		case '+':
			// "one or more of the preceding character" -- only meaningful
			// after something to repeat.
			if b.Len() == len(`\A`) {
				return "", false
			}
			b.WriteString(`+`)
		case '[':
			class, next, ok := branchFilterClass(pattern, i)
			if !ok {
				return "", false
			}
			b.WriteString(class)
			i = next
		case '\\':
			if i+1 >= len(pattern) {
				return "", false
			}
			i++
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(`\z`)
	return b.String(), true
}

// branchFilterClass translates the character class starting at pattern[i]
// (which must be '[') into a regexp class, returning the index of its
// closing ']'. An unterminated or empty class is not translatable.
func branchFilterClass(pattern string, i int) (string, int, bool) {
	end := strings.IndexByte(pattern[i+1:], ']')
	if end < 0 {
		return "", 0, false
	}
	body := pattern[i+1 : i+1+end]
	if body == "" {
		return "", 0, false
	}
	// A leading '!' negates the class, spelled '^' in a regexp.
	if strings.HasPrefix(body, "!") {
		body = "^" + body[1:]
	}
	// Only '\' needs neutralizing inside a class; every other byte is
	// already literal there.
	return "[" + strings.ReplaceAll(body, `\`, `\\`) + "]", i + 1 + end, true
}

// baseProtectedBranches is the hardcoded floor that always applies,
// regardless of whether the dynamic detection in protectedBranches
// succeeds: these names (plus the remote HEAD symref, which listBranches
// already filters out before this is ever consulted) are never eligible
// for deletion.
func baseProtectedBranches() []string {
	return []string{"main", "master", "HEAD"}
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
// A failed default-branch lookup is returned, not swallowed: a repo whose
// default branch is neither main nor master would otherwise be left
// eligible for deletion by a silent error. Callers decide -- reporting can
// carry on, deleting must not.
func protectedBranches(ctx context.Context, repo string) ([]string, error) {
	protected := append(baseProtectedBranches(), workflowTriggerBranches(workflowsDir)...)
	def, err := defaultBranch(ctx, repo)
	if err != nil {
		return protected, fmt.Errorf("default branch of %s: %w", repo, err)
	}
	if def == "" {
		return protected, fmt.Errorf("default branch of %s: empty result", repo)
	}
	return append(protected, def), nil
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

// classifyBranch buckets one branch given its tip commit SHA, the PR
// history whose head it is, and the count of open PRs based ON it:
//
//  1. any OPEN PR from this branch, or any open PR based on it ->
//     review_open_pr (skip, no action, never reported)
//  2. else the most-recently-merged PR (by mergedAt), if any:
//     its headRefOid == tip SHA -> safe_delete (branch content is already
//     in main via the squash commit)
//     anything else (tip moved, or no head SHA recorded) ->
//     review_new_commits_after_merge
//  3. else any CLOSED PR -> review_closed_unmerged
//  4. else -> review_no_pr
func classifyBranch(tipSHA string, prs []prRecord, openBasePRs int) string {
	if openBasePRs > 0 {
		return bucketReviewOpenPR
	}
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
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "git" binary; branch comes from a prior
	// `git for-each-ref` listing of this repo's own remote refs, not
	// external input.
	cmd := exec.CommandContext(ctx, "git", "push", "origin",
		"--force-with-lease="+b.Name+":"+b.TipSHA, "--delete", b.Name)
	if _, err := runCombined(cmd); err != nil {
		// The requested end state may already hold: GitHub's own
		// delete-on-merge, or a human, can remove the ref between
		// enumeration and this push, and git then errors because there is
		// nothing to delete. Reporting that as a failure would claim the
		// branch is still on the remote when it is not.
		if gone, checkErr := remoteBranchAbsent(ctx, b.Name); checkErr == nil && gone {
			return nil
		}
		return err
	}
	return nil
}

// remoteBranchAbsent reports whether branch no longer exists on origin.
func remoteBranchAbsent(ctx context.Context, branch string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "git" binary; branch comes from this repo's
	// own remote refs, not external input.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "origin",
		"refs/heads/"+branch)
	out, err := runCombined(cmd)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// confirmNoOpenPRs re-verifies, immediately before a deletion, that nothing
// open depends on branch -- neither a PR opened FROM it nor one based ON
// it. Any open PR, or any failure to find out, is an error: the caller must
// treat "cannot confirm" exactly like "not safe".
func confirmNoOpenPRs(ctx context.Context, repo, branch string) error {
	prs, err := fetchPRs(ctx, repo, branch)
	if err != nil {
		return fmt.Errorf("recheck head PRs: %w", err)
	}
	for _, pr := range prs {
		if pr.State == "OPEN" {
			return fmt.Errorf("pull request #%d was opened from this branch since classification", pr.Number)
		}
	}
	basePRs, err := fetchOpenBasePRs(ctx, repo, branch)
	if err != nil {
		return fmt.Errorf("recheck base PRs: %w", err)
	}
	if basePRs > 0 {
		return fmt.Errorf("%d open pull request(s) now target this branch as their base", basePRs)
	}
	return nil
}

// fetchPRs returns branch's PR history for PRs opened from this repository.
// Fork PRs are dropped: `gh pr list --head` filters on head branch *name*
// only, so a fork's same-named branch would otherwise be read as this
// branch's history.
func fetchPRs(ctx context.Context, repo, branch string) ([]prRecord, error) {
	// One stalled `gh` call must not hold up every remaining branch: without
	// a per-call deadline a hung subprocess runs until the job timeout kills
	// the whole run with no report at all, which is exactly the "one bad
	// lookup denies results for everything else" failure the lookup_failed
	// bucket exists to prevent. Deadline expiry surfaces as an ordinary
	// lookup error, so the branch lands in lookup_failed like any other.
	ctx, cancel := context.WithTimeout(ctx, prFetchTimeout)
	defer cancel()
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

// fetchOpenBasePRs counts the open pull requests that target branch as
// their BASE. Unlike fetchPRs this deliberately does NOT drop fork PRs: a
// fork's PR based on this branch breaks just as badly when the base
// disappears.
func fetchOpenBasePRs(ctx context.Context, repo, branch string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, prFetchTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "gh" binary; repo/branch are argv elements, not
	// shell input.
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo, "--base", branch, "--state", "open",
		"--json", "number", "--limit", fmt.Sprint(prFetchLimit))
	out, err := runCombined(cmd)
	if err != nil {
		return 0, fmt.Errorf("gh pr list --base: %w", err)
	}
	raw := strings.TrimSpace(out)
	if raw == "" {
		return 0, nil
	}
	var prs []prRecord
	if err := json.Unmarshal([]byte(raw), &prs); err != nil {
		return 0, fmt.Errorf("parse base pr list: %w", err)
	}
	return len(prs), nil
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
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "gh" binary; repo is an argv element, not
	// shell input.
	// The repository is POSITIONAL here: `gh repo view [<repository>]` has
	// no --repo flag and rejects one outright, which would have made every
	// default-branch lookup fail.
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", repo,
		"--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	out, err := runCombined(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// originRemote returns the `origin` remote's host and owner/repo.
func originRemote(ctx context.Context) (host, ownerRepo string, err error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "git" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	out, err := runCombined(cmd)
	if err != nil {
		return "", "", err
	}
	host, ownerRepo = parseRemote(strings.TrimSpace(out))
	return host, ownerRepo, nil
}

// parseRemote splits a git remote URL into its host and its owner/repo,
// covering the https, ssh and scp-like forms git accepts. Either half comes
// back empty when it cannot be determined, which callers must treat as
// "cannot prove a match" -- a filesystem path remote, for instance, has no
// host and so can never be confirmed to be the GitHub repository whose PR
// history was read.
func parseRemote(url string) (host, ownerRepo string) {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, ".git")
	if url == "" {
		return "", ""
	}
	switch {
	case strings.Contains(url, "://"):
		_, rest, _ := strings.Cut(url, "://")
		host, url, _ = strings.Cut(rest, "/")
	case strings.Contains(url, ":"):
		// scp-like "git@host:owner/repo".
		host, url, _ = strings.Cut(url, ":")
	}
	// Strip any "user@" and ":port" decoration from the host.
	if _, after, ok := strings.Cut(host, "@"); ok {
		host = after
	}
	if before, _, ok := strings.Cut(host, ":"); ok {
		host = before
	}
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return host, ""
	}
	owner, name := parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || name == "" {
		return host, ""
	}
	return host, owner + "/" + name
}

// ghHost is the GitHub host `gh` is talking to, so the origin check
// compares the same identity `gh pr list` resolved against.
func ghHost() string {
	if h := strings.TrimSpace(os.Getenv("GH_HOST")); h != "" {
		return h
	}
	return "github.com"
}

// sameRepository reports whether an origin remote names the same repository
// as ownerRepo on the GitHub host in play. Host is part of the identity:
// `gitlab.com/acme/project` and `github.com/acme/project` share an
// owner/repo path and are entirely different repositories.
func sameRepository(originHost, originOwnerRepo, ownerRepo string) bool {
	if originHost == "" || originOwnerRepo == "" || ownerRepo == "" {
		return false
	}
	return strings.EqualFold(originHost, ghHost()) && strings.EqualFold(originOwnerRepo, ownerRepo)
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
