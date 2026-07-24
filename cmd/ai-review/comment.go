package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// commentMarker identifies the sticky AI-review comment so successive runs can
// replace it rather than pile up.
const commentMarker = "<!-- ai-review-5d -->"

// emojiFor maps an outcome to its status emoji for the PR comment.
func emojiFor(o Outcome) string {
	switch o {
	case Approved:
		return "✅"
	case Rejected:
		return "❌"
	case NeedsHumanReview:
		return "🔴"
	case NeedsWork:
		return "⚠️"
	default:
		return "⚠️"
	}
}

// buildComment renders the sticky review comment body.
func buildComment(outcome Outcome, synthesis string, reports []dimensionReport, override bool, triggers []string) string {
	var b strings.Builder
	fmt.Fprintln(&b, commentMarker)
	fmt.Fprintf(&b, "## %s AI Code Review — %s\n\n", emojiFor(outcome), outcome)
	fmt.Fprintf(&b, "> Automated 5-dimension review per [REVIEW.md](REVIEW.md). This is a **required, fail-closed** check: any non-approved outcome blocks merge.\n\n")
	if len(triggers) > 0 {
		fmt.Fprintf(&b, "### 🔴 Mandatory escalation (REVIEW.md §3)\n\nThis diff trips escalation triggers, so the outcome is forced to `needs-human-review` regardless of the dimension findings:\n\n")
		for _, t := range triggers {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		fmt.Fprintln(&b)
	}
	if override {
		fmt.Fprintf(&b, "> ⚠️ `ai-review:override` label present — the check passes on human authority. The override is recorded on this PR.\n\n")
	}
	fmt.Fprintf(&b, "**Synthesis:** %s\n\n", synthesis)
	for _, r := range reports {
		fmt.Fprintf(&b, "<details>\n<summary>%s dimension</summary>\n\n%s\n\n</details>\n\n", r.key, r.text)
	}
	fmt.Fprintf(&b, "<sub>Required gate per REVIEW.md §2/§5.</sub>\n")
	return b.String()
}

// forkComment is posted when a fork PR cannot be auto-reviewed.
func forkComment() string {
	return commentMarker + "\n## 🔴 AI Code Review — needs-human-review\n\n" +
		"> This PR originates from a fork, so the automated review cannot access repository secrets and did not run.\n\n" +
		"A maintainer must review this change and, to merge, apply the `ai-review:override` label (recorded on the PR).\n\n" +
		"<sub>Required gate per REVIEW.md §2/§5.</sub>\n"
}

// overrideComment is posted when the gate passes purely on human authority,
// so every override leaves a sticky, auditable trail on the PR even when no
// automated review could run.
func overrideComment(reason string) string {
	return commentMarker + "\n## ⚠️ AI Code Review — passed by human override\n\n" +
		"> The `ai-review:override` label is present, so this required gate passed on **human authority**: " + reason + ".\n\n" +
		"No automated 5-dimension review backs this result. The override is recorded on this PR as a label.\n\n" +
		"<sub>Required gate per REVIEW.md §2/§5.</sub>\n"
}

// oversizeComment is posted when the diff exceeds the auto-review size limit.
func oversizeComment(size, limit int) string {
	return fmt.Sprintf(commentMarker+"\n## ⚠️ AI Code Review — diff too large\n\n"+
		"> The diff is %d bytes, over the %d-byte auto-review limit, so it was **not** reviewed (the gate refuses to review a truncated diff).\n\n"+
		"Split this PR into smaller reviewable changes, or apply the `ai-review:override` label after a human review.\n\n"+
		"<sub>Required gate per REVIEW.md §2/§5.</sub>\n", size, limit)
}

// postComment upserts the sticky comment and reports whether the result was
// durably recorded. The new comment is posted BEFORE older ones are deleted, so
// a mid-flight failure can never destroy the previous audit record and leave
// nothing in its place.
//
// Callers that merely report a result may ignore the error; the override paths
// must not — see requireComment.
func postComment(c config, body string) error {
	if c.pr == "" || c.repo == "" {
		// No PR context (e.g. merge_group): nothing to post to.
		return nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh not available")
	}

	// Post first so the audit record exists before anything is removed.
	post := exec.Command("gh", "pr", "comment", c.pr, "--repo", c.repo, "--body-file", "-")
	post.Stdin = strings.NewReader(body)
	post.Stdout = os.Stdout
	post.Stderr = os.Stderr
	if err := post.Run(); err != nil {
		fmt.Printf("::warning::failed to post PR comment: %v\n", err)
		return fmt.Errorf("post comment: %w", err)
	}

	// Then prune older marked comments, keeping the one just posted. Pruning is
	// cosmetic — the audit record already exists — so failures are ignored.
	pruneOldComments(c)
	return nil
}

// pruneOldComments removes all but the newest marked sticky comment. Best
// effort by design: the freshly posted comment is the audit record, and losing
// a tidy-up must never turn into a gate failure.
func pruneOldComments(c config) {
	list := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/issues/%s/comments", c.repo, c.pr),
		"--jq", fmt.Sprintf(`.[] | select(.body | startswith("%s")) | .id`, commentMarker))
	out, err := list.Output()
	if err != nil {
		return
	}
	ids := strings.Fields(string(out))
	for _, id := range ids[:max(len(ids)-1, 0)] { // all but the newest
		_ = exec.Command("gh", "api", "-X", "DELETE",
			fmt.Sprintf("repos/%s/issues/comments/%s", c.repo, id)).Run()
	}
}

// requireComment posts a comment that MUST be durably recorded. It reports
// whether the audit record exists. An override that cannot be recorded is not
// an acceptable override: a required check must never pass on human authority
// with no sticky explanation of what was overridden (AIREV-03).
func requireComment(c config, body string) bool {
	err := postComment(c, body)
	if err == nil {
		return true
	}
	fmt.Printf("::error::could not record the override audit comment (%v); refusing to pass on an unrecorded override.\n", err)
	return false
}

// logCommentErr surfaces a failed informational comment as a warning. These
// comments report a result the exit code already carries, so a posting failure
// is visible but never changes the gate's verdict.
func logCommentErr(err error) {
	if err != nil {
		fmt.Printf("::warning::could not post review comment: %v\n", err)
	}
}
