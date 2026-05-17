// Package git provides git functionality.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Worktree represents a git worktree
type Worktree struct {
	Path   string
	Branch string
	IsMain bool
}

// CleanupResult represents the result of cleaning up one worktree
type CleanupResult struct {
	Branch  string
	Path    string
	Removed bool
	Err     error
}

// ListWorktrees lists all git worktrees for the repository at repoPath.
// Returns nil (not an error) if repoPath is not in a git repository.
func ListWorktrees(repoPath string) ([]Worktree, error) {
	// Check if this is a git repo first
	if !IsInGitRepo(repoPath) {
		return nil, nil
	}

	// Find the git root so worktree commands work correctly
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return nil, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	cmd := exec.Command("git", "-C", gitRoot, "worktree", "list", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w\nOutput: %s", err, string(output))
	}

	return parseWorktreeOutput(string(output)), nil
}

// parseWorktreeOutput parses the porcelain output of `git worktree list --porcelain`.
// Each worktree block is separated by a blank line. Fields are:
//
//	worktree <path>
//	HEAD <sha>
//	branch refs/heads/<name>
//	(or "detached" instead of branch)
func parseWorktreeOutput(output string) []Worktree {
	var worktrees []Worktree
	var current Worktree
	isFirst := true

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "worktree "):
			// If we already have a worktree in progress, save it
			if current.Path != "" {
				if isFirst {
					current.IsMain = true
					isFirst = false
				}
				worktrees = append(worktrees, current)
			}
			current = Worktree{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix to get branch name
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "" && current.Path != "":
			// Blank line = end of block
			if isFirst {
				current.IsMain = true
				isFirst = false
			}
			worktrees = append(worktrees, current)
			current = Worktree{}
		}
	}

	// Handle last entry (output may not end with blank line)
	if current.Path != "" {
		if isFirst {
			current.IsMain = true
		}
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// RemoveWorktree removes a single git worktree by path.
// The force parameter controls whether to use --force (removes even with
// uncommitted changes).
func RemoveWorktree(repoPath, worktreePath string, force bool) error {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	args := []string{"-C", gitRoot, "worktree", "remove", worktreePath}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove worktree %s: %w\nOutput: %s", worktreePath, err, string(output))
	}
	return nil
}

// DeleteBranch deletes a local git branch.
// The force parameter controls whether to use -D (force delete) vs -d (safe delete).
func DeleteBranch(repoPath, branchName string, force bool) error {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	flag := "-d"
	if force {
		flag = "-D"
	}

	cmd := exec.Command("git", "-C", gitRoot, "branch", flag, branchName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete branch %s: %w\nOutput: %s", branchName, err, string(output))
	}
	return nil
}

// IsWorktreeClean reports whether the worktree at worktreePath has no
// uncommitted or untracked changes (i.e. `git status --porcelain` is empty).
// A clean worktree can be removed without --force, so this is the gate that
// distinguishes "safe to auto-remove" from "has unsaved work, preserve it".
//
// Any failure to determine status returns (false, err) so callers fail safe
// toward preserving the worktree rather than risking data loss.
func IsWorktreeClean(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status for %s: %w\nOutput: %s",
			worktreePath, err, string(output))
	}
	return len(strings.TrimSpace(string(output))) == 0, nil
}

// IsBranchMerged reports whether branch has been fully merged into baseBranch,
// i.e. branch's tip is an ancestor of baseBranch. This is the same check used
// by RemoveMergedWorktrees.
//
// Caveat: a squash-merged branch is NOT an ancestor of baseBranch (the squash
// commit has a different SHA), so this returns false for squash-merged PRs.
// Callers must treat a false result conservatively (preserve, do not
// force-delete) rather than assuming the work is unmerged and disposable.
func IsBranchMerged(repoPath, branch, baseBranch string) (bool, error) {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return false, fmt.Errorf("not a git repository: %w", err)
	}

	cmd := exec.Command("git", "-C", gitRoot, "merge-base", "--is-ancestor", branch, baseBranch)
	if runErr := cmd.Run(); runErr != nil {
		// Exit code 1 means "not an ancestor" (not merged) — expected, not an error.
		exitErr := &exec.ExitError{}
		if errors.As(runErr, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check merge status of %s into %s: %w",
			branch, baseBranch, runErr)
	}
	return true, nil
}

// AddWorktree creates a new git worktree at the given path on a new branch.
// If branch is empty, a detached HEAD worktree is created from the current HEAD.
func AddWorktree(repoPath, worktreePath, branch string) error {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	var args []string
	if branch != "" {
		args = []string{"-C", gitRoot, "worktree", "add", worktreePath, "-b", branch}
	} else {
		args = []string{"-C", gitRoot, "worktree", "add", "--detach", worktreePath}
	}

	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add worktree at %s: %w\nOutput: %s", worktreePath, err, string(output))
	}
	return nil
}

// RemoveMergedWorktrees removes worktrees whose branches have been merged
// into baseBranch. Returns nil (not an error) if repoPath is not a git repo.
// Per-worktree errors are captured in CleanupResult.Err rather than failing
// the entire operation.
func RemoveMergedWorktrees(repoPath, baseBranch string) ([]CleanupResult, error) {
	worktrees, err := ListWorktrees(repoPath)
	if err != nil {
		return nil, err
	}
	if worktrees == nil {
		// Not a git repo
		return nil, nil
	}

	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return nil, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	var results []CleanupResult

	for _, wt := range worktrees {
		// Skip the main worktree
		if wt.IsMain {
			continue
		}
		// Skip worktrees without a branch (detached HEAD)
		if wt.Branch == "" {
			continue
		}

		result := CleanupResult{
			Branch: wt.Branch,
			Path:   wt.Path,
		}

		// Check if branch is merged into baseBranch using merge-base --is-ancestor
		// If wt.Branch is an ancestor of baseBranch, it has been merged.
		checkCmd := exec.Command("git", "-C", gitRoot, "merge-base", "--is-ancestor", wt.Branch, baseBranch)
		if err := checkCmd.Run(); err != nil {
			// Exit code 1 means not an ancestor (not merged) - this is expected
			// Any other error is a real error
			exitErr := &exec.ExitError{}
			if errors.As(err, &exitErr) {
				// Not merged, skip removal
				results = append(results, result)
				continue
			}
			// Real error
			result.Err = fmt.Errorf("failed to check merge status: %w", err)
			results = append(results, result)
			continue
		}

		// Branch is merged - remove the worktree
		removeCmd := exec.Command("git", "-C", gitRoot, "worktree", "remove", wt.Path)
		if output, err := removeCmd.CombinedOutput(); err != nil {
			result.Err = fmt.Errorf("failed to remove worktree: %w\nOutput: %s", err, string(output))
			results = append(results, result)
			continue
		}

		result.Removed = true
		results = append(results, result)
	}

	return results, nil
}
