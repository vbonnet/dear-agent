// Package safegit blockers.go implements the deterministic PR merge-blocker
// classifier behind cmd/pr-blockers and safe-merge's fast state gate.
//
// Motivation (DEAR retro 2026-08-18, engram-research
// retrospectives/2026-08-18-pr-merge-blocker-guessing.md): agents repeatedly
// guessed at why a PR would not merge and burned tokens hunting phantom code
// issues, when every real blocker is knowable from two deterministic queries:
// `gh pr view --json mergeStateStatus,...` and the reviewThreads GraphQL
// including isOutdated. This file owns the classification so every consumer
// (CLI, safe-merge, skills) reports the same exact blocker and the same exact
// remediation.
package safegit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Verdicts returned by Diagnose.
const (
	VerdictReady   = "READY"   // no blockers; safe-merge may proceed
	VerdictBlocked = "BLOCKED" // one or more named blockers with fixes
	VerdictMerged  = "MERGED"  // nothing to do
	VerdictClosed  = "CLOSED"  // reopening is a human decision
)

// Blocker codes emitted by ClassifyBlockers. Stable identifiers: skills,
// hooks, and tests match on them.
const (
	BlockDraft          = "DRAFT"
	BlockConflicts      = "CONFLICTS"
	BlockFailingCheck   = "FAILING_REQUIRED_CHECK"
	BlockPendingCheck   = "PENDING_REQUIRED_CHECK"
	BlockThreads        = "UNRESOLVED_THREADS"
	BlockChangesReq     = "CHANGES_REQUESTED"
	BlockReviewRequired = "REVIEW_REQUIRED"
	BlockBehind         = "BEHIND"
	BlockUnknown        = "UNKNOWN_BLOCK"
)

// PRState is the deterministic merge-state snapshot from `gh pr view`.
type PRState struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	State            string `json:"state"`     // OPEN | MERGED | CLOSED
	IsDraft          bool   `json:"isDraft"`   //
	Mergeable        string `json:"mergeable"` // MERGEABLE | CONFLICTING | UNKNOWN
	MergeStateStatus string `json:"mergeStateStatus"`
	ReviewDecision   string `json:"reviewDecision"`
	BaseRefName      string `json:"baseRefName"`
	HeadRefName      string `json:"headRefName"`
	HeadRefOid       string `json:"headRefOid"`
}

// Blocker is one exact merge blocker plus its exact remediation.
type Blocker struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

// Diagnosis is the full classifier output for one PR.
type Diagnosis struct {
	Repo     string    `json:"repo"`
	PR       PRState   `json:"pr"`
	Verdict  string    `json:"verdict"`
	Blockers []Blocker `json:"blockers"`
}

// FetchPRState reads the PR's deterministic merge state via gh.
func FetchPRState(ctx context.Context, prNum int, repo string) (PRState, error) {
	out, err := runCommand(exec.CommandContext(ctx, "gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "number,title,url,state,isDraft,mergeable,mergeStateStatus,reviewDecision,baseRefName,headRefName,headRefOid",
	))
	if err != nil {
		return PRState{}, fmt.Errorf("gh pr view failed: %w", err)
	}
	var st PRState
	if err := json.Unmarshal(out, &st); err != nil {
		return PRState{}, fmt.Errorf("parsing pr state: %w", err)
	}
	return st, nil
}

// StateBlockers classifies only the blockers visible from PRState alone (no
// thread or check queries). safe-merge runs this as its fast fail gate; the
// full classifier layers threads and checks on top.
func StateBlockers(st PRState, repo string) []Blocker {
	var out []Blocker
	if st.IsDraft {
		out = append(out, Blocker{
			Code:   BlockDraft,
			Detail: "the PR is a draft; GitHub will not merge drafts",
			Fix: fmt.Sprintf("gh pr ready %d --repo %s  (security/product-behavior/money changes: a human flips and merges per the autonomous-merge policy)",
				st.Number, repo),
		})
	}
	if strings.EqualFold(st.Mergeable, "CONFLICTING") || strings.EqualFold(st.MergeStateStatus, "DIRTY") {
		out = append(out, Blocker{
			Code:   BlockConflicts,
			Detail: fmt.Sprintf("merge conflicts with %s (mergeable=%s)", st.BaseRefName, st.Mergeable),
			Fix:    fmt.Sprintf("safe-rebase onto %s, resolve conflicts, then publish with safe-push", st.BaseRefName),
		})
	}
	if strings.EqualFold(st.MergeStateStatus, "BEHIND") {
		out = append(out, Blocker{
			Code:   BlockBehind,
			Detail: fmt.Sprintf("branch is out of date with %s (required: branches must be up to date before merging)", st.BaseRefName),
			Fix:    fmt.Sprintf("gh pr update-branch %d --repo %s", st.Number, repo),
		})
	}
	return out
}

// ClassifyBlockers produces the ordered, exhaustive blocker list for an open
// PR. Ordering is remediation order: fix content problems (conflicts, checks,
// threads, review) before updating the branch, so one update-branch suffices.
func ClassifyBlockers(st PRState, repo string, threads []ReviewThread, checks []RequiredCheck) []Blocker {
	var out []Blocker
	stateBlockers := StateBlockers(st, repo)
	var behind *Blocker
	for i := range stateBlockers {
		if stateBlockers[i].Code == BlockBehind {
			behind = &stateBlockers[i]
			continue
		}
		out = append(out, stateBlockers[i])
	}

	var failing, pending []string
	for _, c := range checks {
		switch c.Status {
		case RequiredCheckFailing:
			failing = append(failing, c.Name)
		case RequiredCheckPending:
			pending = append(pending, c.Name)
		case RequiredCheckPassing:
		}
	}
	if len(failing) > 0 {
		out = append(out, Blocker{
			Code:   BlockFailingCheck,
			Detail: fmt.Sprintf("required check(s) failing: %s", strings.Join(failing, ", ")),
			Fix:    fmt.Sprintf("fix that check; logs: gh pr checks %d --repo %s (known flakes: rerun once per dear-agent-ci-flakes)", st.Number, repo),
		})
	}
	if len(pending) > 0 {
		out = append(out, Blocker{
			Code:   BlockPendingCheck,
			Detail: fmt.Sprintf("required check(s) still running: %s", strings.Join(pending, ", ")),
			Fix:    fmt.Sprintf("wait: gh pr checks %d --repo %s --watch", st.Number, repo),
		})
	}

	if b := threadBlocker(st, repo, threads); b != nil {
		out = append(out, *b)
	}

	switch strings.ToUpper(st.ReviewDecision) {
	case "CHANGES_REQUESTED":
		out = append(out, Blocker{
			Code:   BlockChangesReq,
			Detail: "a reviewer has requested changes",
			Fix:    "address the review, push, and re-request review",
		})
	case "REVIEW_REQUIRED":
		out = append(out, Blocker{
			Code:   BlockReviewRequired,
			Detail: "an approving review is required and missing",
			Fix:    fmt.Sprintf("obtain an approving review for PR #%d", st.Number),
		})
	}

	if behind != nil {
		out = append(out, *behind)
	}

	// BLOCKED with no detected cause: say so explicitly instead of letting the
	// caller invent one. This is the anti-guessing contract.
	if len(out) == 0 && strings.EqualFold(st.MergeStateStatus, "BLOCKED") {
		out = append(out, Blocker{
			Code:   BlockUnknown,
			Detail: "GitHub reports BLOCKED but no draft/conflict/check/thread/review/behind cause was detected",
			Fix: "re-run pr-blockers (state may be propagating); then compare branch protection " +
				"(docs/branch-protection.md) against `gh api repos/" + repo + "/rules/branches/" + st.BaseRefName +
				"`. Do NOT hunt for code problems: this classifier is exhaustive over GitHub's merge state.",
		})
	}
	return out
}

// threadBlocker folds unresolved review threads (outdated included) into one
// blocker. Outdated threads are called out explicitly: the GitHub UI collapses
// them and default queries omit them, yet required conversation resolution
// still counts them; that invisibility is the single most common cause of
// phantom merge-blocker hunts.
func threadBlocker(st PRState, repo string, threads []ReviewThread) *Blocker {
	var lines []string
	outdated := 0
	for _, t := range threads {
		if t.IsResolved {
			continue
		}
		tag := ""
		if t.IsOutdated {
			outdated++
			tag = " (outdated)"
		}
		author := t.Author
		if author == "" {
			author = "unknown"
		}
		where := t.Path
		if where == "" {
			where = "(no file)"
		}
		if t.ID != "" {
			lines = append(lines, fmt.Sprintf("@%s %s%s [%s]", author, where, tag, t.ID))
		} else {
			lines = append(lines, fmt.Sprintf("@%s %s%s", author, where, tag))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	detail := fmt.Sprintf("%d unresolved review thread(s)", len(lines))
	if outdated > 0 {
		detail += fmt.Sprintf(", %d outdated (collapsed under 'Show outdated' in the UI; they STILL block required conversation resolution)", outdated)
	}
	detail += ": " + strings.Join(lines, "; ")
	parts := strings.SplitN(repo, "/", 2)
	owner, name := parts[0], ""
	if len(parts) == 2 {
		name = parts[1]
	}
	return &Blocker{
		Code:   BlockThreads,
		Detail: detail,
		Fix: fmt.Sprintf("address each thread in code, then close it with its reason using the "+
			"thread ID in brackets above: resolve-review-threads reply-resolve <threadId> "+
			"\"Fixed - <what changed>\" (list them again any time with: "+
			"resolve-review-threads list %s %s %d; sweep the answered ones with: "+
			"resolve-review-threads resolve-all %s %s %d [author], which refuses threads "+
			"nobody replied to)",
			owner, name, st.Number, owner, name, st.Number),
	}
}

// Diagnose runs the full deterministic classification for one PR.
func Diagnose(ctx context.Context, prNum int, repo string) (Diagnosis, error) {
	st, err := FetchPRState(ctx, prNum, repo)
	if err != nil {
		return Diagnosis{}, err
	}
	d := Diagnosis{Repo: repo, PR: st}
	switch strings.ToUpper(st.State) {
	case "MERGED":
		d.Verdict = VerdictMerged
		return d, nil
	case "CLOSED":
		d.Verdict = VerdictClosed
		return d, nil
	}

	threads, err := ListReviewThreads(ctx, prNum, repo)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("listing review threads: %w", err)
	}
	checks, err := ProjectRequiredChecks(ctx, prNum, repo)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("projecting required checks: %w", err)
	}

	d.Blockers = ClassifyBlockers(st, repo, threads, checks)
	if len(d.Blockers) == 0 {
		d.Verdict = VerdictReady
	} else {
		d.Verdict = VerdictBlocked
	}
	return d, nil
}
