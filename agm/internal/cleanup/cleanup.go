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

// GitOps abstracts git operations for testability.
type GitOps interface {
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	DeleteBranch(repoPath, branchName string, force bool) error
	// PreserveBranch reports whether a branch must survive cleanup, and
	// why. Deleting a branch here is unrecoverable, so this gate sits in
	// the interface rather than inside DeleteBranch: a caller that swaps in
	// its own GitOps must make the preservation decision explicitly.
	PreserveBranch(repoPath, branchName string) (preserve bool, reason string)
	// PreserveWorktree reports whether a worktree must survive cleanup, and
	// why. This is the liveness half of the same rule: RemoveWorktree is
	// called with force, so an unexamined removal discards uncommitted and
	// ignored work that no ref points at and nothing can recover. It sits in
	// the interface for the same reason PreserveBranch does.
	PreserveWorktree(repoPath, worktreePath string) (preserve bool, reason string)
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

// PreserveBranch defers to the shared branch-preservation oracle, so tracked
// cleanup, archive cleanup, and the worktree sweep all answer this question
// the same way.
func (RealGitOps) PreserveBranch(repoPath, branchName string) (bool, string) {
	v := gitpkg.PreserveLocalBranch(repoPath, branchName)
	return v.Preserve, v.Reason
}

// PreserveWorktree answers the liveness question from the filesystem. It
// preserves a worktree that has staged, unstaged, or untracked changes, and it
// preserves one whose status cannot be determined at all: "unknown" is not
// evidence that there is nothing to lose, and the removal it gates is forced
// and unrecoverable.
func (RealGitOps) PreserveWorktree(_, worktreePath string) (bool, string) {
	dirty, err := gitpkg.HasUncommittedChanges(worktreePath)
	if err != nil {
		return true, fmt.Sprintf("worktree status could not be determined: %v", err)
	}
	if dirty {
		return true, "worktree has uncommitted or untracked changes"
	}
	return false, ""
}

// Result holds the outcome of a session cleanup operation.
type Result struct {
	WorktreesRemoved int `json:"worktrees_removed"`
	BranchesDeleted  int `json:"branches_deleted"`
	// BranchesPreserved names each branch cleanup declined to delete, with
	// the reason, so a preserved branch is visible rather than merely absent
	// from the deleted count.
	BranchesPreserved []string `json:"branches_preserved,omitempty"`
	// WorktreesPreserved names each worktree cleanup declined to remove, with
	// the reason, so preserved recovery state is visible rather than merely
	// absent from the removed count.
	WorktreesPreserved   []string `json:"worktrees_preserved,omitempty"`
	TmpFilesRemoved      int      `json:"tmp_files_removed"`
	InterruptFlagCleared bool     `json:"interrupt_flag_cleared"`
	Errors               []string `json:"errors,omitempty"`
}

// SessionResources cleans up resources associated with a session during archive.
// It performs three cleanup tasks:
//  1. Remove git worktrees tracked in the database for this session
//  2. Delete the recorded branches only after their owning worktrees are gone
//  3. Remove /tmp/build-SESSION* files
//
// All cleanup is best-effort: errors are logged and collected but do not halt the process.
func SessionResources(ctx context.Context, sessionName string, store WorktreeStore, git GitOps, logger *slog.Logger) *Result {
	if logger == nil {
		logger = slog.Default()
	}

	result := &Result{}

	// 1. Clean up worktrees tracked in the database. Branch deletion is
	// authorized only by the branch recorded on a worktree that was removed (or
	// was already absent), never by a merely matching session name.
	branchesByRepo := map[string]map[string]struct{}{}
	rememberRemovedBranch := func(repoPath, branch string) {
		if repoPath == "" || branch == "" {
			return
		}
		if branchesByRepo[repoPath] == nil {
			branchesByRepo[repoPath] = map[string]struct{}{}
		}
		branchesByRepo[repoPath][branch] = struct{}{}
	}
	if store != nil {
		worktrees, err := store.ListWorktreesBySession(ctx, sessionName)
		if err != nil {
			msg := fmt.Sprintf("failed to list worktrees for session %s: %v", sessionName, err)
			logger.Warn(msg)
			result.Errors = append(result.Errors, msg)
		} else {
			for _, wt := range worktrees {
				// Check if worktree directory still exists
				if _, statErr := os.Stat(wt.WorktreePath); os.IsNotExist(statErr) {
					logger.Info("Worktree already gone, untracking", "path", wt.WorktreePath)
					_ = store.UntrackWorktree(ctx, wt.WorktreePath)
					result.WorktreesRemoved++
					rememberRemovedBranch(wt.RepoPath, wt.Branch)
					continue
				}

				// Liveness gate before a forced removal. Removing a
				// worktree that still holds uncommitted or ignored work
				// destroys state no ref points at, so a preserved worktree
				// stays tracked (the next cleanup must be able to retry it)
				// and keeps its branch, which is the other half of the same
				// recovery state.
				if preserve, reason := git.PreserveWorktree(wt.RepoPath, wt.WorktreePath); preserve {
					logger.Info("Preserving worktree during session cleanup",
						"path", wt.WorktreePath, "repo", wt.RepoPath, "reason", reason)
					result.WorktreesPreserved = append(result.WorktreesPreserved,
						fmt.Sprintf("%s (%s)", wt.WorktreePath, reason))
					continue
				}

				// Remove the git worktree (force is safe past the gate above)
				if err := git.RemoveWorktree(wt.RepoPath, wt.WorktreePath, true); err != nil {
					msg := fmt.Sprintf("failed to remove worktree %s: %v", wt.WorktreePath, err)
					logger.Warn(msg)
					result.Errors = append(result.Errors, msg)
				} else {
					logger.Info("Removed worktree", "path", wt.WorktreePath)
					result.WorktreesRemoved++
					rememberRemovedBranch(wt.RepoPath, wt.Branch)
				}

				// Untrack in database regardless of removal success
				_ = store.UntrackWorktree(ctx, wt.WorktreePath)
			}
		}
	}

	// 2. Delete only branches positively attributed by removed worktree
	// records, and only when nothing worth keeping lives on them. Attribution
	// alone is not authorization: the archive path can decide to preserve a
	// branch with an open PR and then reach this function, which used to
	// force-delete the very ref the caller had just reported as kept.
	for repoPath, branches := range branchesByRepo {
		for branch := range branches {
			if preserve, reason := git.PreserveBranch(repoPath, branch); preserve {
				logger.Info("Preserving branch during session cleanup",
					"branch", branch, "repo", repoPath, "reason", reason)
				result.BranchesPreserved = append(result.BranchesPreserved,
					fmt.Sprintf("%s (%s)", branch, reason))
				continue
			}
			if err := git.DeleteBranch(repoPath, branch, true); err != nil {
				// Branch may not exist or already be deleted — this is expected.
				logger.Debug("Could not delete branch", "branch", branch, "repo", repoPath, "error", err)
			} else {
				logger.Info("Deleted branch", "branch", branch, "repo", repoPath)
				result.BranchesDeleted++
			}
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
