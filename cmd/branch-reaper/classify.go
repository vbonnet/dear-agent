// classify.go holds the pure decision logic: which bucket a branch belongs
// in, and which branches are protected from ever being considered.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

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
