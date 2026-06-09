package git

import (
	"context"
	"os"
	"os/exec"
	"strconv"
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
	state, ok := PRState(repoPath, branch)
	if !ok {
		return false, false
	}
	return state == "MERGED", true
}

// PRState returns the state of the pull request whose head is exactly
// `branch` — one of "MERGED", "OPEN", "CLOSED". `known` is false whenever
// the answer cannot be positively established: gh not installed, not
// authenticated, no PR for the branch, the resolved PR's head is some other
// branch, a timeout, or any network/parse error. Callers MUST treat
// (_, false) as "do not act on this".
//
// It shares PRMergedState's hard timeout, prompt-disabling and
// exact-head-match guard, so it is equally safe on a hook path. The sweep
// uses the full state (not just merged?) so it can tell an OPEN PR
// (in-flight work — keep) from a worktree with no PR at all (husk).
func PRState(repoPath, branch string) (state string, known bool) {
	if branch == "" {
		return "", false
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", false
	}
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return "", false
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
		return "", false
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
	if len(parts) != 2 {
		return "", false
	}
	st, head := parts[0], parts[1]
	if head != branch {
		// gh resolved a PR whose head is not this exact branch — refuse to
		// act on an ambiguous match.
		return "", false
	}
	return st, true
}

// OpenPRForBranch reports the number of an OPEN pull request whose head is
// exactly `branch`, if one exists.
//
// "known" is false whenever the answer cannot be positively established — gh
// missing/unauthenticated, no PR for the branch, the resolved PR's head is a
// different branch, a timeout, or any network/parse error. It uses the same
// hard-bounded, non-prompting invocation as PRMergedState so it is safe on
// latency-sensitive paths.
func OpenPRForBranch(repoPath, branch string) (number int, known bool) {
	if branch == "" {
		return 0, false
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return 0, false
	}
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), prCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view",
		"--json", "state,headRefName,number",
		"--jq", `.state + "\t" + .headRefName + "\t" + (.number|tostring)`,
		"--", branch)
	cmd.Dir = gitRoot
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 3)
	if len(parts) != 3 {
		return 0, false
	}
	state, head, numStr := parts[0], parts[1], parts[2]
	if head != branch || state != "OPEN" {
		return 0, false
	}
	n, convErr := strconv.Atoi(numStr)
	if convErr != nil {
		return 0, false
	}
	return n, true
}
