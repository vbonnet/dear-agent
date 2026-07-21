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
	default:
		return "⚠️"
	}
}

// buildComment renders the sticky review comment body.
func buildComment(outcome Outcome, synthesis string, reports []dimensionReport, override bool) string {
	var b strings.Builder
	fmt.Fprintln(&b, commentMarker)
	fmt.Fprintf(&b, "## %s AI Code Review — %s\n\n", emojiFor(outcome), outcome)
	fmt.Fprintf(&b, "> Automated 5-dimension review per [REVIEW.md](REVIEW.md). This is a **required, fail-closed** check: any non-approved outcome blocks merge.\n\n")
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

// oversizeComment is posted when the diff exceeds the auto-review size limit.
func oversizeComment(size, limit int) string {
	return fmt.Sprintf(commentMarker+"\n## ⚠️ AI Code Review — diff too large\n\n"+
		"> The diff is %d bytes, over the %d-byte auto-review limit, so it was **not** reviewed (the gate refuses to review a truncated diff).\n\n"+
		"Split this PR into smaller reviewable changes, or apply the `ai-review:override` label after a human review.\n\n"+
		"<sub>Required gate per REVIEW.md §2/§5.</sub>\n", size, limit)
}

// postComment upserts the sticky comment via the gh CLI. It is best-effort:
// any failure is logged but never changes the review's exit code, so a comment
// outage can neither block nor unblock a merge.
func postComment(c config, body string) {
	if c.pr == "" || c.repo == "" {
		return
	}
	if _, err := exec.LookPath("gh"); err != nil {
		fmt.Println("::warning::gh not available; skipping PR comment.")
		return
	}
	// Delete any prior marked comment, then post the new one.
	del := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/issues/%s/comments", c.repo, c.pr),
		"--jq", fmt.Sprintf(`.[] | select(.body | startswith("%s")) | .id`, commentMarker))
	if out, err := del.Output(); err == nil {
		for _, id := range strings.Fields(string(out)) {
			_ = exec.Command("gh", "api", "-X", "DELETE",
				fmt.Sprintf("repos/%s/issues/comments/%s", c.repo, id)).Run()
		}
	}
	post := exec.Command("gh", "pr", "comment", c.pr, "--repo", c.repo, "--body-file", "-")
	post.Stdin = strings.NewReader(body)
	post.Stdout = os.Stdout
	post.Stderr = os.Stderr
	if err := post.Run(); err != nil {
		fmt.Printf("::warning::failed to post PR comment: %v\n", err)
	}
}
