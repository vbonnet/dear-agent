package mergeloop

import (
	"fmt"
	"strings"
)

// BuildPrompt produces the self-contained instruction for a spawned agent. The
// spawned session has no memory of the loop, so the prompt carries everything:
// the PR, the failure, the rules (no --no-verify/--force, gtimeout, worktrees),
// and an explicit Definition of Done.
func BuildPrompt(repo string, pr PR, kind AgentKind) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a merge-loop worker. Drive PR #%d in %s to a mergeable state.\n\n", pr.Number, repo)
	fmt.Fprintf(&b, "PR: #%d %q\nBranch: %s\nHead: %s\n\n", pr.Number, pr.Title, pr.HeadRefName, pr.HeadSHA)

	switch kind {
	case AgentFixCI:
		b.WriteString("TASK: A required CI check is failing. Diagnose the root cause from the\n")
		b.WriteString("CI logs, fix the code, and push so CI re-runs green.\n\n")
		if fails := failingNames(pr); fails != "" {
			fmt.Fprintf(&b, "Failing required checks: %s\n\n", fails)
		}
	case AgentResolveConflict:
		b.WriteString("TASK: This PR has merge conflicts against its base. Rebase the branch onto\n")
		b.WriteString("the latest base and resolve every conflict intelligently (preserve the\n")
		b.WriteString("PR's intent; do not drop unrelated changes from base), then push.\n\n")
	}

	b.WriteString("RULES (mandatory):\n")
	b.WriteString("- Work only in a git worktree under ~/worktrees/**. Never edit ~/src/** directly.\n")
	b.WriteString("- Use `safe-push` for every push. Keep other git/network operations non-interactive and bounded.\n")
	b.WriteString("- NEVER use --no-verify or --force / --force-with-lease.\n")
	b.WriteString("- Commit incrementally with conventional messages; uncommitted work is lost work.\n")
	b.WriteString("- VERIFICATION GATE (mandatory): Run go test / make preflight locally and\n")
	b.WriteString("  confirm green before pushing or declaring done. Do not rely on CI alone.\n")
	b.WriteString("- If you hit the SAME failure twice, stop and report it — do not retry endlessly.\n")
	b.WriteString("- Do NOT merge the PR yourself; the merge loop merges it once checks are green.\n\n")

	b.WriteString("DEFINITION OF DONE:\n")
	switch kind {
	case AgentFixCI:
		b.WriteString("- [ ] Root cause identified and fixed in code\n")
		b.WriteString("- [ ] Changes committed and pushed to the PR branch\n")
		b.WriteString("- [ ] All required CI checks pass on the new head\n")
	case AgentResolveConflict:
		b.WriteString("- [ ] Branch rebased on latest base; all conflicts resolved\n")
		b.WriteString("- [ ] No unrelated base changes dropped\n")
		b.WriteString("- [ ] Changes committed and pushed; PR no longer conflicting\n")
	}
	return b.String()
}

func failingNames(pr PR) string {
	var names []string
	for _, c := range pr.requiredChecks() {
		if c.Verdict == CheckFail {
			names = append(names, c.Name)
		}
	}
	return strings.Join(names, ", ")
}
