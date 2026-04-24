// Package ops — archive_cleanup.go provides post-archive cleanup orchestration.
//
// When a session is archived, resources such as git worktrees, sandbox
// directories, and session branches accumulate on disk. This file
// provides CleanupAfterArchive which runs all cleanup steps and logs
// each action to ~/.agm/logs/cleanup.jsonl for auditability.
package ops

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CleanupAction records a single cleanup step in cleanup.jsonl.
type CleanupAction struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"session_id"`
	SessionName string    `json:"session_name"`
	Action      string    `json:"action"`
	Target      string    `json:"target,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// CleanupResult summarises all post-archive cleanup work.
type CleanupResult struct {
	WorktreesRemoved    int  `json:"worktrees_removed"`
	WorktreesPruned     bool `json:"worktrees_pruned"`
	SandboxRemoved      bool `json:"sandbox_removed"`
	BranchDeleted       bool `json:"branch_deleted"`
	SandboxBranchDeleted bool `json:"sandbox_branch_deleted"`
}

// CleanupAfterArchive performs best-effort resource cleanup after a session
// has been archived. It removes the git worktree, prunes orphaned worktrees,
// deletes the session branch (if fully merged), and removes the sandbox
// directory. Every action is logged to ~/.agm/logs/cleanup.jsonl.
//
// If keepSandbox is true the sandbox directory is preserved for debugging.
func CleanupAfterArchive(sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName string, keepSandbox bool) *CleanupResult {
	result := &CleanupResult{}
	logger := newCleanupLogger()

	// 1. Remove the git worktree (force to handle uncommitted changes).
	if worktreePath != "" {
		err := removeWorktreeCmd(repoPath, worktreePath)
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "remove_worktree",
			Target:      worktreePath,
			Success:     err == nil,
			Error:       errStr(err),
		})
		if err == nil {
			result.WorktreesRemoved++
		} else {
			slog.Warn("Failed to remove worktree during archive cleanup",
				"path", worktreePath, "error", err)
		}
	}

	// 2. Prune orphaned worktrees from the repo.
	if repoPath != "" {
		err := pruneWorktrees(repoPath)
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "prune_worktrees",
			Target:      repoPath,
			Success:     err == nil,
			Error:       errStr(err),
		})
		if err == nil {
			result.WorktreesPruned = true
		} else {
			slog.Warn("Failed to prune worktrees during archive cleanup",
				"repo", repoPath, "error", err)
		}
	}

	// 3. Force-delete the session branch. We use -D (force) because archived
	// session branches are almost never fast-forward merged (squash-merge via
	// PR is the norm), so the safe -d would silently fail in most cases.
	if branchName != "" && repoPath != "" {
		err := forceDeleteBranch(repoPath, branchName)
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "delete_branch",
			Target:      branchName,
			Success:     err == nil,
			Error:       errStr(err),
		})
		if err == nil {
			result.BranchDeleted = true
		} else {
			slog.Debug("Branch not deleted (may not exist)",
				"branch", branchName, "error", err)
		}
	}

	// 3b. Delete the agm/<sessionID> sandbox branch if it exists.
	if sessionID != "" && repoPath != "" {
		sandboxBranch := "agm/" + sessionID
		err := forceDeleteBranch(repoPath, sandboxBranch)
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "delete_sandbox_branch",
			Target:      sandboxBranch,
			Success:     err == nil,
			Error:       errStr(err),
		})
		if err == nil {
			result.SandboxBranchDeleted = true
		} else {
			slog.Debug("Sandbox branch not deleted (may not exist)",
				"branch", sandboxBranch, "error", err)
		}
	}

	// 4. Remove the sandbox directory (unless --keep-sandbox).
	if sandboxPath != "" && !keepSandbox {
		removed := cleanupSandboxDir(sessionID, sandboxPath)
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "remove_sandbox",
			Target:      sandboxPath,
			Success:     removed,
			Error:       boolErrStr(!removed, "sandbox removal failed or not found"),
		})
		result.SandboxRemoved = removed
	} else if keepSandbox && sandboxPath != "" {
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "keep_sandbox",
			Target:      sandboxPath,
			Success:     true,
		})
	}

	return result
}

// removeWorktreeCmd runs `git worktree remove --force`.
func removeWorktreeCmd(repoPath, worktreePath string) error {
	if repoPath == "" {
		return fmt.Errorf("repoPath is empty")
	}
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, string(output))
	}
	return nil
}

// pruneWorktrees runs `git worktree prune` to clean up stale worktree metadata.
func pruneWorktrees(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "prune")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune: %w\n%s", err, string(output))
	}
	return nil
}

// forceDeleteBranch force-deletes a local branch using `git branch -D`.
// Archived sessions' branches are almost never fast-forward merged (squash-merge
// via PR is the norm), so the safe -d would silently fail in most cases.
func forceDeleteBranch(repoPath, branchName string) error {
	cmd := exec.Command("git", "-C", repoPath, "branch", "-D", branchName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D %s: %w\n%s", branchName, err, string(output))
	}
	return nil
}

// --- cleanup.jsonl logger ---

type cleanupLogger struct {
	path string
}

func newCleanupLogger() *cleanupLogger {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	logPath := filepath.Join(home, ".agm", "logs", "cleanup.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		slog.Warn("Failed to create cleanup log directory", "error", err)
		return nil
	}
	return &cleanupLogger{path: logPath}
}

func logAction(logger *cleanupLogger, action CleanupAction) {
	if logger == nil {
		return
	}
	if action.Timestamp.IsZero() {
		action.Timestamp = time.Now()
	}
	data, err := json.Marshal(action)
	if err != nil {
		return
	}
	data = append(data, '\n')
	f, err := os.OpenFile(logger.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

// errStr returns "" for nil errors, otherwise the error string.
func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// boolErrStr returns msg if cond is true, otherwise "".
func boolErrStr(cond bool, msg string) string {
	if cond {
		return msg
	}
	return ""
}
