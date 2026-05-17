// Package cleanup provides session resource cleanup during archive lifecycle.
package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
)

// WorktreeRecord mirrors dolt.WorktreeRecord to avoid circular imports.
type WorktreeRecord struct {
	WorktreePath string
	RepoPath     string
	Branch       string
	SessionName  string
}

// WorktreeStore abstracts worktree database operations for testability.
type WorktreeStore interface {
	ListWorktreesBySession(ctx context.Context, sessionName string) ([]WorktreeRecord, error)
	UntrackWorktree(ctx context.Context, worktreePath string) error
}

// defaultBaseBranch is the branch a session branch must be merged into
// before its worktree and local branch are considered safe to delete.
const defaultBaseBranch = "main"

// GitOps abstracts git operations for testability.
type GitOps interface {
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	DeleteBranch(repoPath, branchName string, force bool) error
	// IsWorktreeClean reports whether the worktree has no uncommitted or
	// untracked changes. Errors fail safe toward "not clean" (preserve).
	IsWorktreeClean(worktreePath string) (bool, error)
	// IsBranchMerged reports whether branch is fully merged into baseBranch.
	// Errors fail safe toward "not merged" (preserve).
	IsBranchMerged(repoPath, branch, baseBranch string) (bool, error)
}

// RealGitOps implements GitOps using real git commands.
type RealGitOps struct{}

// RemoveWorktree removes a git worktree via the gitpkg helper.
func (RealGitOps) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	return gitpkg.RemoveWorktree(repoPath, worktreePath, force)
}

// DeleteBranch deletes a git branch via the gitpkg helper.
func (RealGitOps) DeleteBranch(repoPath, branchName string, force bool) error {
	return gitpkg.DeleteBranch(repoPath, branchName, force)
}

// IsWorktreeClean reports whether the worktree has no uncommitted changes.
func (RealGitOps) IsWorktreeClean(worktreePath string) (bool, error) {
	return gitpkg.IsWorktreeClean(worktreePath)
}

// IsBranchMerged reports whether branch is fully merged into baseBranch.
func (RealGitOps) IsBranchMerged(repoPath, branch, baseBranch string) (bool, error) {
	return gitpkg.IsBranchMerged(repoPath, branch, baseBranch)
}

// Result holds the outcome of a session cleanup operation.
type Result struct {
	WorktreesRemoved     int      `json:"worktrees_removed"`
	WorktreesPreserved   int      `json:"worktrees_preserved"`
	BranchesDeleted      int      `json:"branches_deleted"`
	TmpFilesRemoved      int      `json:"tmp_files_removed"`
	InterruptFlagCleared bool     `json:"interrupt_flag_cleared"`
	Errors               []string `json:"errors,omitempty"`
	// Warnings records non-fatal "preserved instead of deleted" notices,
	// each including the worktree path so an operator can find the work.
	Warnings []string `json:"warnings,omitempty"`
}

// SessionResources cleans up resources associated with a session during archive.
//
// Worktree disposition is decided per worktree, never destructively:
//
//   - worktree directory already gone     → untrack, attempt safe branch delete
//   - has uncommitted/untracked changes    → PRESERVE, warn with the path
//   - clean but branch not merged to main  → PRESERVE, warn with the path
//     (covers genuinely-unpushed work and the squash-merge ambiguity)
//   - clean AND branch merged into main    → remove worktree + delete local branch
//
// Removal uses a non-force `git worktree remove` and branch deletion uses the
// safe `git branch -d`, so git itself is the last line of defence: an unmerged
// branch cannot be deleted even if the logic above is wrong. This is the
// opposite of the previous behaviour, which force-removed every tracked
// worktree and force-deleted (`-D`) its branch, silently destroying
// uncommitted and unpushed work and driving operators to disable the hook.
//
// It also removes /tmp/build-SESSION* files and clears the interrupt flag.
// All cleanup is best-effort: errors are logged and collected but do not halt
// the process.
func SessionResources(ctx context.Context, sessionName string, store WorktreeStore, git GitOps, logger *slog.Logger) *Result {
	if logger == nil {
		logger = slog.Default()
	}

	result := &Result{}

	// 1. Decide each tracked worktree's fate without ever destroying work.
	//    Only worktrees that were actually disposed of (gone, or clean+merged
	//    and removed) make their repo eligible for safe local-branch deletion.
	reposToReap := map[string]bool{}
	if store != nil {
		worktrees, err := store.ListWorktreesBySession(ctx, sessionName)
		if err != nil {
			msg := fmt.Sprintf("failed to list worktrees for session %s: %v", sessionName, err)
			logger.Warn(msg)
			result.Errors = append(result.Errors, msg)
		} else {
			for _, wt := range worktrees {
				if disposeWorktree(ctx, wt, store, git, result, logger) {
					reposToReap[wt.RepoPath] = true
				}
			}
		}
	}

	// 2. Delete the session's local branch only in repos where its worktree
	//    was safely disposed of. force=false → `git branch -d`, which git
	//    itself refuses for an unmerged branch (defence in depth).
	for repoPath := range reposToReap {
		if err := git.DeleteBranch(repoPath, sessionName, false); err != nil {
			// Expected when the branch is unmerged (git -d refused), already
			// deleted, or never existed — keep it and move on.
			logger.Debug("Did not delete branch", "branch", sessionName, "repo", repoPath, "error", err)
		} else {
			logger.Info("Deleted merged branch", "branch", sessionName, "repo", repoPath)
			result.BranchesDeleted++
		}
	}

	// 3. Clean up /tmp/build-SESSION* files
	cleanupTmpFiles(sessionName, result, logger)

	// 4. Clear interrupt flag (if any)
	if err := interrupt.Clear(interrupt.DefaultDir(), sessionName); err != nil {
		msg := fmt.Sprintf("failed to clear interrupt flag for %s: %v", sessionName, err)
		logger.Warn(msg)
		result.Errors = append(result.Errors, msg)
	} else {
		logger.Info("Cleared interrupt flag", "session", sessionName)
		result.InterruptFlagCleared = true
	}

	return result
}

// disposeWorktree decides the fate of a single tracked worktree and applies it.
// It returns true when the worktree was disposed of (already gone, or
// clean+merged and removed) so the caller may safely delete its local branch.
// Dirty or unmerged worktrees are preserved (and left tracked) so a later run,
// or a human, can deal with the unsaved work — never destroyed here.
func disposeWorktree(
	ctx context.Context,
	wt WorktreeRecord,
	store WorktreeStore,
	git GitOps,
	result *Result,
	logger *slog.Logger,
) bool {
	// Already gone: nothing to remove, just stop tracking it.
	if _, statErr := os.Stat(wt.WorktreePath); os.IsNotExist(statErr) {
		logger.Info("Worktree already gone, untracking", "path", wt.WorktreePath)
		_ = store.UntrackWorktree(ctx, wt.WorktreePath)
		result.WorktreesRemoved++
		return true
	}

	branch := wt.Branch
	if branch == "" {
		branch = wt.SessionName
	}

	clean, err := git.IsWorktreeClean(wt.WorktreePath)
	if err != nil {
		// Could not determine state — fail safe: preserve.
		preserveWorktree(result, logger, wt.WorktreePath, branch,
			fmt.Sprintf("could not determine git status (%v)", err))
		return false
	}
	if !clean {
		preserveWorktree(result, logger, wt.WorktreePath, branch,
			"uncommitted or untracked changes present")
		return false
	}

	merged, err := git.IsBranchMerged(wt.RepoPath, branch, defaultBaseBranch)
	if err != nil {
		preserveWorktree(result, logger, wt.WorktreePath, branch,
			fmt.Sprintf("could not determine merge status (%v)", err))
		return false
	}
	if !merged {
		// Clean but not merged into base: either genuinely unpushed work or a
		// squash-merged PR we cannot distinguish git-locally. Either way, do
		// not destroy it.
		preserveWorktree(result, logger, wt.WorktreePath, branch,
			fmt.Sprintf("branch not merged into %s (unpushed work or squash-merge)", defaultBaseBranch))
		return false
	}

	// Clean AND merged: safe to remove with a non-force worktree remove.
	if err := git.RemoveWorktree(wt.RepoPath, wt.WorktreePath, false); err != nil {
		msg := fmt.Sprintf("failed to remove clean+merged worktree %s: %v", wt.WorktreePath, err)
		logger.Warn(msg)
		result.Errors = append(result.Errors, msg)
	} else {
		logger.Info("Removed clean, merged worktree", "path", wt.WorktreePath, "branch", branch)
		result.WorktreesRemoved++
	}
	// Untrack regardless: a merged worktree should not stay in the DB even if
	// the filesystem removal raced or partially failed.
	_ = store.UntrackWorktree(ctx, wt.WorktreePath)
	return true
}

// preserveWorktree records that a worktree was deliberately kept, emitting a
// warning that includes the path so an operator can locate the unsaved work.
func preserveWorktree(result *Result, logger *slog.Logger, path, branch, reason string) {
	logger.Warn("Preserving worktree (not safe to auto-remove)",
		"path", path, "branch", branch, "reason", reason)
	result.WorktreesPreserved++
	result.Warnings = append(result.Warnings,
		fmt.Sprintf("preserved worktree %s (branch %s): %s", path, branch, reason))
}

// cleanupTmpFiles removes /tmp/build-SESSION* files.
func cleanupTmpFiles(sessionName string, result *Result, logger *slog.Logger) {
	tmpDir := os.TempDir()
	prefix := "build-" + sessionName

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		msg := fmt.Sprintf("failed to read tmp dir: %v", err)
		logger.Warn(msg)
		result.Errors = append(result.Errors, msg)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) {
			path := filepath.Join(tmpDir, name)
			if err := os.Remove(path); err != nil {
				msg := fmt.Sprintf("failed to remove tmp file %s: %v", path, err)
				logger.Warn(msg)
				result.Errors = append(result.Errors, msg)
			} else {
				logger.Info("Removed tmp file", "path", path)
				result.TmpFilesRemoved++
			}
		}
	}
}
