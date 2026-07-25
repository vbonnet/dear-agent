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

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
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
	WorktreesRemoved     int  `json:"worktrees_removed"`
	PrimaryWorktreeKept  bool `json:"primary_worktree_kept"`
	WorktreesPruned      bool `json:"worktrees_pruned"`
	SandboxRemoved       bool `json:"sandbox_removed"`
	BranchDeleted        bool `json:"branch_deleted"`
	SandboxBranchDeleted bool `json:"sandbox_branch_deleted"`
}

type sandboxCleanupFunc func(sessionID, mergedPath string) bool

// CleanupAfterArchive performs best-effort resource cleanup after a session
// has been archived. It removes the git worktree, prunes orphaned worktrees,
// deletes the session branch (if fully merged), and removes the sandbox
// directory. Every action is logged to ~/.agm/logs/cleanup.jsonl.
//
// If keepSandbox is true the sandbox directory is preserved for debugging.
func CleanupAfterArchive(sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName string, keepSandbox bool) *CleanupResult {
	return cleanupAfterArchive(
		sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName,
		keepSandbox, cleanupSandboxDir,
	)
}

//nolint:gocyclo // linear cleanup transaction keeps attribution, worktree, branch, and sandbox decisions in execution order
func cleanupAfterArchive(sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName string, keepSandbox bool, cleanSandbox sandboxCleanupFunc) *CleanupResult {
	result := &CleanupResult{}
	logger := newCleanupLogger()
	primaryWorktree := false
	branchDeletionSafe := false
	candidatePath := worktreePath
	if candidatePath == "" {
		candidatePath = repoPath
	}

	// 1. Classify the checkout before authorizing any destructive cleanup. A
	// missing explicit worktree path falls back to the manifest project path,
	// but that fallback is evidence for preservation only: it never authorizes
	// worktree or branch deletion.
	if candidatePath != "" {
		identity, classifyErr := classifyWorktree(candidatePath)
		switch {
		case classifyErr != nil:
			logAction(logger, CleanupAction{
				SessionID:   sessionID,
				SessionName: sessionName,
				Action:      "classify_worktree",
				Target:      candidatePath,
				Success:     false,
				Error:       classifyErr.Error(),
			})
			slog.Warn("Preserving unclassified worktree and branch during archive cleanup",
				"path", candidatePath, "error", classifyErr)
		case identity.IsPrimary:
			primaryWorktree = true
			result.PrimaryWorktreeKept = true
			logAction(logger, CleanupAction{
				SessionID:   sessionID,
				SessionName: sessionName,
				Action:      "keep_primary_worktree",
				Target:      identity.Root,
				Success:     true,
			})
			slog.Info("Preserving primary checkout during archive cleanup", "path", identity.Root)
		case worktreePath == "":
			logAction(logger, CleanupAction{
				SessionID:   sessionID,
				SessionName: sessionName,
				Action:      "keep_context_worktree",
				Target:      identity.Root,
				Success:     true,
			})
			slog.Info("Preserving context worktree without explicit ownership metadata", "path", identity.Root)
		default:
			err := removeWorktreeCmd(repoPath, identity.Root)
			logAction(logger, CleanupAction{
				SessionID:   sessionID,
				SessionName: sessionName,
				Action:      "remove_worktree",
				Target:      identity.Root,
				Success:     err == nil,
				Error:       errStr(err),
			})
			if err == nil {
				result.WorktreesRemoved++
				branchDeletionSafe = identity.Branch != "" && identity.Branch == branchName
				if !branchDeletionSafe {
					slog.Info("Preserving branch not owned by removed worktree",
						"requested_branch", branchName, "worktree_branch", identity.Branch)
				}
			} else {
				slog.Warn("Failed to remove worktree during archive cleanup",
					"path", identity.Root, "error", err)
			}
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
	if branchName != "" && repoPath != "" && !primaryWorktree && branchDeletionSafe {
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
	} else if branchName != "" && repoPath != "" {
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "keep_unowned_branch",
			Target:      branchName,
			Success:     true,
		})
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
		removed := cleanSandbox != nil && cleanSandbox(sessionID, sandboxPath)
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

// classifyWorktree resolves a possibly nested project directory to its actual
// worktree root, then proves whether that inventory entry is the primary
// checkout. os.SameFile keeps symlink and spelling aliases identity-safe.
type worktreeIdentity struct {
	Root      string
	Branch    string
	IsPrimary bool
}

func classifyWorktree(path string) (worktreeIdentity, error) {
	worktreeRoot, err := gitpkg.WorktreeRoot(path)
	if err != nil {
		return worktreeIdentity{}, err
	}
	worktrees, err := gitpkg.ListWorktrees(worktreeRoot)
	if err != nil {
		return worktreeIdentity{}, fmt.Errorf("list worktrees for %s: %w", worktreeRoot, err)
	}
	if len(worktrees) == 0 {
		return worktreeIdentity{}, fmt.Errorf("no worktree inventory found for %s", worktreeRoot)
	}
	pathInfo, err := os.Stat(worktreeRoot)
	if err != nil {
		return worktreeIdentity{}, fmt.Errorf("stat candidate worktree: %w", err)
	}
	for _, worktree := range worktrees {
		inventoryInfo, statErr := os.Stat(worktree.Path)
		if statErr != nil {
			continue
		}
		if os.SameFile(pathInfo, inventoryInfo) {
			return worktreeIdentity{
				Root:      worktreeRoot,
				Branch:    worktree.Branch,
				IsPrimary: worktree.IsMain,
			}, nil
		}
	}
	return worktreeIdentity{}, fmt.Errorf("worktree root %s is absent from Git inventory", worktreeRoot)
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
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
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
	f, err := os.OpenFile(logger.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
