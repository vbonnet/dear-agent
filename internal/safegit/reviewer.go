// Package safegit reviewer.go implements the P4 expected-reviewer gate: every
// reviewer listed under expected_reviewers in .safe-merge.yml must have a review
// that is newer than the head SHA push. Three outcomes are distinguished and all
// non-pass outcomes are surfaced — a reviewer that never showed up is never a
// silent pass:
//
//   - PASS:          a fresh review from the reviewer exists.
//   - STALE:         a review exists but predates the last push.
//   - NEVER_ARRIVED: no review from the reviewer within its timeout.
//
// While still inside the timeout window the outcome is PENDING (also an error,
// so a one-shot run blocks); watch mode retries until PASS or NEVER_ARRIVED.
package safegit

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// checkExpectedReviewers fetches the PR's reviews and head push time and
// validates them against the configured expected reviewers.
func checkExpectedReviewers(prNum int, repo string, reviewers []ExpectedReviewer, now time.Time) error {
	out, err := runCommand(exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "headRefOid,commits,reviews",
	))
	if err != nil {
		return fmt.Errorf("gh pr view failed: %w", err)
	}
	return parseExpectedReviewers(out, reviewers, now)
}

// reviewerView mirrors the gh pr view JSON needed for the reviewer gate. Reviews
// carry submittedAt so we can compare each against the head push time.
type reviewerView struct {
	HeadRefOid string `json:"headRefOid"`
	Commits    []struct {
		CommittedDate time.Time `json:"committedDate"`
	} `json:"commits"`
	Reviews []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State       string    `json:"state"`
		SubmittedAt time.Time `json:"submittedAt"`
	} `json:"reviews"`
}

// parseExpectedReviewers validates the gh pr view JSON against the configured
// reviewers. The head push time is taken as the newest commit's committedDate —
// the same anchor the soak gate uses — so a review counts as fresh only if it
// postdates the last code change. Exported (lower-case, package-internal) for
// testing via mocked gh output. Returns the first non-pass outcome it finds so
// the operator gets one clear, actionable message at a time.
func parseExpectedReviewers(data []byte, reviewers []ExpectedReviewer, now time.Time) error {
	if len(reviewers) == 0 {
		return nil
	}

	var pr reviewerView
	if err := json.Unmarshal(data, &pr); err != nil {
		return fmt.Errorf("parsing pr view: %w", err)
	}

	// Head push time proxy: newest commit on the PR.
	var pushTime time.Time
	for _, c := range pr.Commits {
		if c.CommittedDate.After(pushTime) {
			pushTime = c.CommittedDate
		}
	}

	for _, want := range reviewers {
		// Newest review from this login.
		var newest time.Time
		found := false
		for _, r := range pr.Reviews {
			if !sameReviewer(r.Author.Login, want.Login) {
				continue
			}
			// Any review state counts — Gemini Code Assist submits its findings
			// as COMMENTED, never APPROVED, so requiring APPROVED here would
			// block every Gemini-reviewed PR forever. Freshness (below) is the
			// only additional constraint.
			found = true
			if r.SubmittedAt.After(newest) {
				newest = r.SubmittedAt
			}
		}

		timeout := time.Duration(want.Timeout()) * time.Minute

		if found {
			if want.RequireNewerThanPush && !pushTime.IsZero() && !newest.After(pushTime) {
				return fmt.Errorf(
					"STALE: review by %s predates the last push (review %s, push %s) — re-request a review",
					want.Login,
					newest.UTC().Format(time.RFC3339),
					pushTime.UTC().Format(time.RFC3339),
				)
			}
			// Fresh review (or freshness not required) → this reviewer passes.
			continue
		}

		// No review from this reviewer yet.
		var waited time.Duration
		if !pushTime.IsZero() {
			waited = now.Sub(pushTime)
		}
		if waited >= timeout {
			return fmt.Errorf(
				"NEVER_ARRIVED: %s has not reviewed within %s of the last push — escalate or re-request",
				want.Login, timeout)
		}
		return fmt.Errorf(
			"PENDING: waiting for %s to review (%s of %s elapsed since last push)",
			want.Login, waited.Round(time.Second), timeout)
	}

	return nil
}

// sameReviewer compares two GitHub logins tolerating the "[bot]" suffix that
// some GitHub surfaces append to GitHub-App accounts. The reviews API returns
// the bare login (e.g. "gemini-code-assist"), while operators commonly
// configure "gemini-code-assist[bot]" in .safe-merge.yml. Without this
// normalization gate 4 would never match the bot's review and block the merge
// indefinitely — the exact failure mode this fix addresses.
func sameReviewer(a, b string) bool {
	return strings.TrimSuffix(a, "[bot]") == strings.TrimSuffix(b, "[bot]")
}

// reviewerLogins returns the configured reviewer logins, for log/diagnostic use.
func reviewerLogins(reviewers []ExpectedReviewer) string {
	logins := make([]string, 0, len(reviewers))
	for _, r := range reviewers {
		logins = append(logins, r.Login)
	}
	return strings.Join(logins, ", ")
}
