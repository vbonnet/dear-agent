// Package git provides git functionality.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Worktree represents a git worktree
type Worktree struct {
	Path       string
	Branch     string
	IsMain     bool
	Locked     bool
	LockReason string
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
		case line == "locked":
			current.Locked = true
			current.LockReason = ""
		case strings.HasPrefix(line, "locked "):
			current.Locked = true
			reason := strings.TrimPrefix(line, "locked ")
			if unquoted, err := strconv.Unquote(reason); err == nil {
				reason = unquoted
			}
			current.LockReason = reason
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

// HasUncommittedChanges reports whether the worktree at worktreePath has any
// staged, unstaged, or untracked changes (i.e. `git status --porcelain` is
// non-empty). A true result means the worktree is NOT safe to remove.
//
// On any error determining status it returns (true, err): the caller must
// treat "unknown" as "dirty" so a status failure can never cause data loss.
func HasUncommittedChanges(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, fmt.Errorf("failed to get status for %s: %w\nOutput: %s", worktreePath, err, string(output))
	}
	return strings.TrimSpace(string(output)) != "", nil
}

// ResolveBaseRef returns the ref that worktree branches are measured against
// when deciding whether they hold unmerged work. Resolution order:
//
//  1. origin/HEAD (the remote's default branch)
//  2. origin/main
//  3. main
//
// Returns "" if none of these resolve, which the caller MUST treat as
// "cannot determine base" — and therefore must not remove the worktree.
func ResolveBaseRef(repoPath string) string {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return ""
	}
	if out, err := exec.Command("git", "-C", gitRoot,
		"symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		if ref := strings.TrimSpace(string(out)); ref != "" {
			return ref
		}
	}
	for _, ref := range []string{"origin/main", "main"} {
		if exec.Command("git", "-C", gitRoot,
			"rev-parse", "--verify", "--quiet", ref).Run() == nil {
			return ref
		}
	}
	return ""
}

// CommitsAhead returns the number of commits reachable from ref but not from
// base (equivalent to `git rev-list --count base..ref`). A return of 0 means
// ref carries no unmerged commits relative to base.
//
// On any error it returns (-1, err): the caller MUST treat a negative count
// as "has unmerged work" so a counting failure can never cause data loss.
//
// Note: a squash-merged branch reports a positive count even though its
// content is already on base. This is intentional for a cleanup gate — it
// keeps such worktrees rather than risking removal of work that only looks
// merged. Reclaiming squash-merged trees requires the explicit PR merge proof
// in `agm worktree sweep`, not this conservative path.
func CommitsAhead(repoPath, ref, base string) (int, error) {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return -1, fmt.Errorf("not a git repository: %w", err)
	}
	if ref == "" || base == "" {
		return -1, fmt.Errorf("ref and base must both be set (ref=%q base=%q)", ref, base)
	}
	out, err := exec.Command("git", "-C", gitRoot,
		"rev-list", "--count", base+".."+ref).Output()
	if err != nil {
		return -1, fmt.Errorf("failed to count commits %s..%s: %w", base, ref, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1, fmt.Errorf("unexpected rev-list output %q: %w", string(out), err)
	}
	return n, nil
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

// WorktreeAddOptions configures worktree creation.
type WorktreeAddOptions struct {
	Lock       bool
	LockReason string
	Detach     bool
}

// AddWorktree creates a new git worktree at the given path on a new branch.
// If branch is empty, a detached HEAD worktree is created from the current HEAD.
func AddWorktree(repoPath, worktreePath, branch string) error {
	return AddWorktreeWithOpts(repoPath, worktreePath, branch, WorktreeAddOptions{})
}

// AddWorktreeWithOpts creates a new git worktree with options such as locking.
func AddWorktreeWithOpts(repoPath, worktreePath, branch string, opts WorktreeAddOptions) error {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	var args []string
	if branch != "" && !opts.Detach {
		args = []string{"-C", gitRoot, "worktree", "add", worktreePath, "-b", branch}
	} else {
		args = []string{"-C", gitRoot, "worktree", "add", "--detach", worktreePath}
	}

	if opts.Lock {
		args = append(args, "--lock")
		if opts.LockReason != "" {
			args = append(args, "--reason", opts.LockReason)
		}
	}

	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add worktree at %s: %w\nOutput: %s", worktreePath, err, string(output))
	}
	return nil
}

// LockWorktree locks a git worktree with an optional reason.
func LockWorktree(repoPath, worktreePath, reason string) error {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	args := []string{"-C", gitRoot, "worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	args = append(args, worktreePath)

	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to lock worktree %s: %w\nOutput: %s", worktreePath, err, string(output))
	}
	return nil
}

// UnlockWorktree unlocks a git worktree by path.
// It is idempotent: if the worktree is already unlocked, it returns nil.
func UnlockWorktree(repoPath, worktreePath string) error {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	cmd := exec.Command("git", "-C", gitRoot, "worktree", "unlock", worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		outStr := string(output)
		if strings.Contains(outStr, "is not locked") {
			return nil
		}
		return fmt.Errorf("failed to unlock worktree %s: %w\nOutput: %s", worktreePath, err, outStr)
	}
	return nil
}

// IsExplicitPreserve reports whether a worktree lock reason indicates explicit
// operator preservation (e.g. "preserve", "keep", "hold", "pin", "manual", "do-not-reap", "wip").
func IsExplicitPreserve(reason string) bool {
	r := strings.ToLower(strings.TrimSpace(reason))
	if r == "" {
		return false
	}
	prefixes := []string{
		"preserve",
		"keep",
		"hold",
		"pin",
		"manual",
		"do-not-reap",
		"do_not_reap",
		"wip",
	}
	for _, p := range prefixes {
		if r == p || strings.HasPrefix(r, p+":") || strings.HasPrefix(r, p+"-") || strings.HasPrefix(r, p+" ") {
			return true
		}
	}
	return false
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

		// Branch is merged - remove the worktree.
		// If locked, check if explicitly preserved; otherwise unlock before removing.
		if wt.Locked {
			if IsExplicitPreserve(wt.LockReason) {
				continue
			}
			if err := UnlockWorktree(gitRoot, wt.Path); err != nil {
				result.Err = fmt.Errorf("failed to unlock worktree before removal: %w", err)
				results = append(results, result)
				continue
			}
		}

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
