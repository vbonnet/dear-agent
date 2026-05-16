package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// prCheckTimeout caps a single `gh pr view` call. The reaper additionally
// bounds total PR-check time across a pass; this per-call cap keeps any one
// hung lookup from eating that whole budget.
const prCheckTimeout = 8 * time.Second

// PRMergedState reports whether the pull request whose head is `branch` has
// been merged on the remote.
//
// The second return value, "known", is false whenever the answer cannot be
// positively established — gh not installed, not authenticated, no PR for the
// branch, the resolved PR's head is some other branch, a timeout, or any
// network/parse error. Callers MUST treat unknown as "not safe to remove":
// this is the authoritative signal that lets the reaper reclaim a
// squash-merged worktree (which looks commits-ahead to plain git) WITHOUT
// risking deletion of work that only appears merged.
//
// It never prompts: GIT_TERMINAL_PROMPT and gh's own prompts are disabled and
// the call is hard-bounded by prCheckTimeout, so it is safe on the Stop-hook
// path that must never block session exit.
func PRMergedState(repoPath, branch string) (merged bool, known bool) {
	if branch == "" {
		return false, false
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return false, false
	}
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return false, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), prCheckTimeout)
	defer cancel()

	// Flags first, then `--`, then the branch as the final positional: the
	// `--` terminates flag parsing so a branch that begins with `-` can
	// never be misread as a flag. (Order matters — `--` before the flags
	// would make --json/--jq positional and break the call.)
	cmd := exec.CommandContext(ctx, "gh", "pr", "view",
		"--json", "state,headRefName",
		"--jq", `.state + "\t" + .headRefName`,
		"--", branch)
	cmd.Dir = gitRoot
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	out, err := cmd.Output()
	if err != nil {
		// No PR for this branch, auth failure, timeout, offline, etc.
		return false, false
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
	if len(parts) != 2 {
		return false, false
	}
	state, head := parts[0], parts[1]
	if head != branch {
		// gh resolved a PR whose head is not this exact branch — refuse to
		// act on an ambiguous match.
		return false, false
	}
	return state == "MERGED", true
}
