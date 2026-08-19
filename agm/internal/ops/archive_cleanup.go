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
	WorktreesRemoved    int  `json:"worktrees_removed"`
	PrimaryWorktreeKept bool `json:"primary_worktree_kept"`
	WorktreesPruned     bool `json:"worktrees_pruned"`
	SandboxRemoved      bool `json:"sandbox_removed"`
	// SandboxRemovalFailed is true when a sandbox directory was expected to
	// exist and be removable but removal did not succeed (as opposed to
	// there being no sandbox to remove at all, which is not a failure).
	// Archive callers must surface this visibly rather than only reporting
	// it via the (often unread) SandboxRemoved count — a silently-failed
	// sandbox reap otherwise under-reports and leaks disk.
	SandboxRemovalFailed bool `json:"sandbox_removal_failed,omitempty"`
	// SandboxRemovalReason carries the underlying refusal (live process,
	// remaining mount, filesystem error) so the caller can print something
	// actionable instead of pointing at a log that only holds generic text.
	SandboxRemovalReason string `json:"sandbox_removal_reason,omitempty"`
	BranchDeleted        bool   `json:"branch_deleted"`
	// BranchKeptOpenPR is true when the session branch was NOT force-deleted
	// because it has a confirmed open PR — the branch is in-flight work, not
	// abandoned, so deleting the local ref (while the remote/PR head
	// survives) would be needlessly destructive.
	BranchKeptOpenPR bool `json:"branch_kept_open_pr,omitempty"`
	// BranchKeptReason records why a branch was preserved when the reason is
	// something other than a plain open PR, such as commits that exist on no
	// remote. Empty when nothing was preserved.
	BranchKeptReason     string `json:"branch_kept_reason,omitempty"`
	SandboxBranchDeleted bool   `json:"sandbox_branch_deleted"`
	// SandboxBranchKept is true when the agm/<sessionID> branch survived
	// because it carries an open PR or unpushed commits. This branch used to
	// be deleted unconditionally, outside the guard that covered the session
	// branch.
	SandboxBranchKept bool `json:"sandbox_branch_kept,omitempty"`
}

// preserveBranch is the shared branch-preservation oracle, indirected through
// a package variable so tests can drive the classification without a gh
// binary or a remote.
var preserveBranch = func(repoPath, branch string) (bool, string) {
	v := gitpkg.PreserveLocalBranch(repoPath, branch)
	return v.Preserve, v.Reason
}

type sandboxCleanupFunc func(sessionID, mergedPath string) (removed bool, existed bool, reason string)

// CleanupAfterArchive performs best-effort resource cleanup after a session
// has been archived. It removes the git worktree, prunes orphaned worktrees,
// deletes the session branch (if fully merged and it has no open PR), and
// removes the sandbox directory. Every action is logged to
// ~/.agm/logs/cleanup.jsonl.
//
// If keepSandbox is true the sandbox directory is preserved for debugging.
// If hasOpenPR is true the local session branch is preserved even when it
// would otherwise be eligible for force-deletion (see BranchKeptOpenPR).
func CleanupAfterArchive(sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName string, keepSandbox, hasOpenPR bool) *CleanupResult {
	return cleanupAfterArchive(
		sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName,
		keepSandbox, hasOpenPR, cleanupSandboxDir,
	)
}

//nolint:gocyclo // linear cleanup transaction keeps attribution, worktree, branch, and sandbox decisions in execution order
func cleanupAfterArchive(sessionID, sessionName, worktreePath, repoPath, sandboxPath, branchName string, keepSandbox, hasOpenPR bool, cleanSandbox sandboxCleanupFunc) *CleanupResult {
	result := &CleanupResult{}
	logger := newCleanupLogger()
	primaryWorktree := false
	branchDeletionSafe := false
	cleanupRepoPath := repoPath
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
			survivingRepoPath, resolveErr := gitpkg.MainWorktreePath(identity.Root)
			if resolveErr != nil {
				cleanupRepoPath = ""
				logAction(logger, CleanupAction{
					SessionID:   sessionID,
					SessionName: sessionName,
					Action:      "resolve_surviving_repo",
					Target:      identity.Root,
					Success:     false,
					Error:       resolveErr.Error(),
				})
				slog.Warn("Could not resolve surviving repository path before removing linked worktree",
					"path", identity.Root, "error", resolveErr)
			} else {
				cleanupRepoPath = survivingRepoPath
				err := removeWorktreeCmd(cleanupRepoPath, identity.Root)
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
	}

	// 2. Prune orphaned worktrees from the repo.
	if cleanupRepoPath != "" {
		err := pruneWorktrees(cleanupRepoPath)
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "prune_worktrees",
			Target:      cleanupRepoPath,
			Success:     err == nil,
			Error:       errStr(err),
		})
		if err == nil {
			result.WorktreesPruned = true
		} else {
			slog.Warn("Failed to prune worktrees during archive cleanup",
				"repo", cleanupRepoPath, "error", err)
		}
	}

	// 3. Force-delete the session branch. We use -D (force) because archived
	// session branches are almost never fast-forward merged (squash-merge via
	// PR is the norm), so the safe -d would silently fail in most cases.
	//
	// A confirmed open PR overrides deletion even when the worktree ownership
	// check (branchDeletionSafe) says it would otherwise be safe: an open PR
	// means the branch is in-flight, not abandoned, and stripping the local
	// ref is needlessly destructive (the retro's "resolved := merged |
	// has-open-PR | explicitly-abandoned" rule applied to cleanup, not just
	// the archive gate).
	//
	// hasOpenPR is the caller's verdict, taken from the pre-archive
	// verification. It is not the only reason to keep a branch: the shared
	// oracle also preserves a branch whose commits exist on no remote, and
	// fails closed when the PR state cannot be established. Both must hold
	// for a deletion to proceed.
	keepBranch, keepReason := false, ""
	if branchName != "" && cleanupRepoPath != "" && !primaryWorktree && branchDeletionSafe {
		if hasOpenPR {
			keepBranch, keepReason = true, "open PR"
		} else if preserve, reason := preserveBranch(cleanupRepoPath, branchName); preserve {
			keepBranch, keepReason = true, reason
		}
	}
	switch {
	case branchName != "" && cleanupRepoPath != "" && !primaryWorktree && branchDeletionSafe && !keepBranch:
		err := forceDeleteBranch(cleanupRepoPath, branchName)
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
	case branchName != "" && cleanupRepoPath != "" && !primaryWorktree && branchDeletionSafe && keepBranch:
		result.BranchKeptOpenPR = true
		result.BranchKeptReason = keepReason
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "keep_branch_open_pr",
			Target:      branchName,
			Success:     true,
			Error:       keepReason,
		})
		slog.Info("Preserving local branch during archive cleanup",
			"branch", branchName, "reason", keepReason)
	case branchName != "" && cleanupRepoPath != "":
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "keep_unowned_branch",
			Target:      branchName,
			Success:     true,
		})
	}

	// 3b. Delete the agm/<sessionID> sandbox branch if it exists.
	//
	// This is the common sandbox-session path, and it used to bypass the
	// branch guard entirely: hasOpenPR was resolved for the session branch,
	// so a sandbox session on the conventional agm/<sessionID> branch had its
	// open-PR ref force-deleted regardless. Ask the oracle about this exact
	// branch instead.
	if sessionID != "" && cleanupRepoPath != "" && branchExists(cleanupRepoPath, "agm/"+sessionID) {
		sandboxBranch := "agm/" + sessionID
		if preserve, reason := preserveBranch(cleanupRepoPath, sandboxBranch); preserve {
			result.SandboxBranchKept = true
			logAction(logger, CleanupAction{
				SessionID:   sessionID,
				SessionName: sessionName,
				Action:      "keep_sandbox_branch",
				Target:      sandboxBranch,
				Success:     true,
				Error:       reason,
			})
			slog.Info("Preserving sandbox branch during archive cleanup",
				"branch", sandboxBranch, "reason", reason)
		} else {
			err := forceDeleteBranch(cleanupRepoPath, sandboxBranch)
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
	}

	// 4. Remove the sandbox directory (unless --keep-sandbox).
	if sandboxPath != "" && !keepSandbox {
		var removed, existed bool
		var reason string
		if cleanSandbox != nil {
			removed, existed, reason = cleanSandbox(sessionID, sandboxPath)
		}
		failed := existed && !removed
		// Record the underlying refusal, not a generic sentence. The archive
		// command points the operator at this log file for the detail, so a
		// placeholder here leaves them with nothing to diagnose.
		if failed && reason == "" {
			reason = "sandbox existed but removal failed for an unreported reason"
		}
		logAction(logger, CleanupAction{
			SessionID:   sessionID,
			SessionName: sessionName,
			Action:      "remove_sandbox",
			Target:      sandboxPath,
			Success:     removed,
			Error:       boolErrStr(failed, reason),
		})
		result.SandboxRemoved = removed
		result.SandboxRemovalFailed = failed
		result.SandboxRemovalReason = reason
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

// branchExists reports whether a local branch is present. The sandbox-branch
// step asks first so a session that never had one is neither reported as
// deleted nor as preserved.
func branchExists(repoPath, branch string) bool {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return cmd.Run() == nil
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
