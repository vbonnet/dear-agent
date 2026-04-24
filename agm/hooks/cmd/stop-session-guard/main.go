// stop-session-guard validates git cleanliness, worktree hygiene, and branch
// state before allowing a Claude Code session to exit.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func main() {
	os.Exit(stophook.RunWithTimeout(10*time.Second, run))
}

func run() int {
	input, err := stophook.ReadInput(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[stop-session-guard] failed to read input: %v\n", err)
		return 0 // fail open
	}

	dir := input.Cwd
	if dir == "" {
		return 0
	}

	if !stophook.IsGitRepo(dir) {
		return 0 // not a git repo, nothing to check
	}

	result := &stophook.Result{HookName: "stop-session-guard"}

	checkGitStatus(result, dir)
	checkUnpushed(result, dir)
	checkWorktrees(result, dir)
	checkBranches(result, dir)

	result.Report()
	return result.ExitCode()
}

func checkGitStatus(r *stophook.Result, dir string) {
	dirty, err := stophook.GitStatus(dir)
	if err != nil {
		r.Warn("git-status", fmt.Sprintf("could not check: %v", err), "")
		return
	}
	if len(dirty) == 0 {
		r.Pass("git-status", "clean working directory")
		return
	}
	r.Block("git-status",
		fmt.Sprintf("%d uncommitted file(s)", len(dirty)),
		"commit or stash changes before exiting")
}

func checkUnpushed(r *stophook.Result, dir string) {
	unpushed, err := stophook.GitUnpushedCommits(dir)
	if err != nil {
		r.Warn("unpushed", fmt.Sprintf("could not check: %v", err), "")
		return
	}
	if len(unpushed) == 0 {
		r.Pass("unpushed", "all commits pushed")
		return
	}
	r.Warn("unpushed",
		fmt.Sprintf("%d unpushed commit(s)", len(unpushed)),
		"push changes: git push")
}

func checkWorktrees(r *stophook.Result, dir string) {
	worktrees, err := stophook.GitWorktrees(dir)
	if err != nil {
		r.Warn("worktrees", fmt.Sprintf("could not check: %v", err), "")
		return
	}

	// Check each non-bare worktree for uncommitted changes
	dirtyCount := 0
	for _, wt := range worktrees {
		if wt.Bare {
			continue
		}
		dirty, err := stophook.GitStatus(wt.Path)
		if err != nil {
			continue
		}
		if len(dirty) > 0 {
			dirtyCount++
			fmt.Fprintf(os.Stderr, "  [worktree] %s has %d uncommitted file(s)\n", wt.Path, len(dirty))
		}
	}

	if dirtyCount > 0 {
		r.Block("worktrees",
			fmt.Sprintf("%d worktree(s) with uncommitted changes", dirtyCount),
			"commit or discard changes in worktrees")
	} else {
		r.Pass("worktrees", "all worktrees clean")
	}
}

func checkBranches(r *stophook.Result, dir string) {
	extra, err := stophook.GitExtraBranches(dir)
	if err != nil {
		r.Warn("branches", fmt.Sprintf("could not check: %v", err), "")
		return
	}
	if len(extra) == 0 {
		r.Pass("branches", "no extra branches")
		return
	}
	r.Warn("branches",
		fmt.Sprintf("%d extra branch(es): %v", len(extra), extra),
		"clean up merged branches")
}
