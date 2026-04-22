// Package reaper provides reaper functionality.
package reaper

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"context"

	"github.com/vbonnet/dear-agent/agm/internal/cleanup"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const (
	// PromptDetectionTimeout is how long to wait for Claude to return to prompt
	// before falling back to timer-based waiting. This should be generous enough
	// to handle slow responses but not so long that stuck sessions block indefinitely.
	// Reduced to 90 seconds after fixing GetRawLine blocking bug. Most commands complete
	// within seconds; if no prompt after 90s, likely stuck and fallback should trigger.
	PromptDetectionTimeout = 90 * time.Second

	// PaneCloseTimeout is how long to wait for the tmux pane to close after sending /exit.
	// Claude should exit quickly after receiving /exit command, but we allow extra time for
	// cleanup operations. Increased from 30s to 60s to give Claude time to run SessionEnd hooks.
	PaneCloseTimeout = 60 * time.Second

	// FallbackWaitTime is used when prompt detection fails or times out.
	// This is a conservative estimate of how long Claude might take to finish
	// a response and return to the prompt.
	// Reduced to 60 seconds to avoid excessive waiting for stuck sessions.
	FallbackWaitTime = 60 * time.Second

	// SIGTERMGracePeriod is how long to wait after sending SIGTERM before escalating to SIGKILL.
	SIGTERMGracePeriod = 10 * time.Second

	// PostKillPaneTimeout is how long to wait for pane to close after killing the process.
	PostKillPaneTimeout = 5 * time.Second

	// ReaperTimeout is the maximum wall-clock time a reaper may run before
	// it is considered a zombie itself. If exceeded, Phase 1 (graceful /exit)
	// is abandoned: tmux is force-killed and the session is archived directly.
	ReaperTimeout = 10 * time.Minute
)

// Reaper manages the async archival process for a AGM session
// It waits for Claude to return to prompt, sends /exit, and archives the session
type Reaper struct {
	SessionName string
	SessionsDir string
	SocketPath  string
	logger      *slog.Logger
}

// New creates a new Reaper for the given session
func New(sessionName, sessionsDir string) *Reaper {
	return &Reaper{
		SessionName: sessionName,
		SessionsDir: sessionsDir,
		SocketPath:  tmux.GetSocketPath(),
		logger:      logging.DefaultLogger(),
	}
}

// Run executes the full two-phase reaper sequence:
//
// Phase 1 — Stop the process (must complete before Phase 2):
//  1. Wait for Claude to return to prompt (prompt detection)
//  2. Send /exit command to exit Claude
//  3. Wait for pane to close (timeout: 60s)
//  4. If timeout: send SIGTERM to pane process, wait 10s
//  5. If still alive: send SIGKILL, wait 5s
//  6. Kill tmux session as final cleanup
//
// Phase 2 — Archive in Dolt (only after pane is confirmed dead):
//  7. Archive session (update Dolt + cleanup resources)
//
// This ordering prevents zombie panes: a session is never marked archived
// in Dolt while its tmux pane is still alive.
func (r *Reaper) Run() error {
	startTime := time.Now()
	r.logger.Info("Starting reaper sequence (two-phase)", "session", r.SessionName, "timeout", ReaperTimeout)

	// Step 0: Safety guard check - don't /exit if a human is present
	guardResult := safety.Check(r.SessionName, safety.GuardOptions{
		SkipUninitialized: true, // reaper only runs on initialized sessions
		SkipMidResponse:   true, // reaper already waits for prompt
	})
	if !guardResult.Safe {
		r.logger.Warn("Safety guard blocked reaper", "violations", guardResult.Error())
		return fmt.Errorf("safety guard blocked reaper on session '%s':\n%s", r.SessionName, guardResult.Error())
	}

	// Step 1: Mark session as "reaping" in Dolt BEFORE touching tmux.
	// This ensures that if the reaper is killed mid-sequence (e.g., machine
	// restart), the GC startup scan will find the session in "reaping" state
	// with a dead tmux and archive it automatically.
	r.logger.Info("Marking session as reaping in Dolt")
	if err := r.markReaping(); err != nil {
		// Non-fatal: proceed with reaping even if we can't mark state.
		// The GC startup scan will catch it as "stopped" anyway.
		r.logger.Warn("Failed to mark session as reaping (will proceed)", "error", err)
	}

	// ── Phase 1: Stop the process (gated by ReaperTimeout) ─────────────
	zombieDetected := r.stopProcess(startTime)

	// Step 6: Kill tmux session as final cleanup (idempotent, always runs)
	r.logger.Info("Killing tmux session (final cleanup)")
	tmux.KillSession(r.SessionName)

	// Verify pane is actually gone
	if active, _ := tmux.IsPaneActive(r.SessionName); active {
		r.logger.Error("CRITICAL: pane still alive after all kill attempts — refusing to archive")
		return fmt.Errorf("pane for session '%s' is still alive after SIGKILL + kill-session; refusing to archive to prevent zombie", r.SessionName)
	}

	// ── Phase 2: Archive in Dolt (pane is confirmed dead) ──────────────

	r.logger.Info("Archiving session (pane confirmed dead)")
	if err := r.archiveSession(); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	if zombieDetected {
		r.logger.Warn("Session force-archived due to reaper timeout (zombie detected)",
			"session", r.SessionName, "elapsed", time.Since(startTime))
	}
	r.logger.Info("Session archived successfully")

	return nil
}

// stopProcess runs Phase 1: graceful shutdown with timeout protection.
// Returns true if the ReaperTimeout was exceeded (zombie detected),
// meaning /exit was skipped and tmux should be force-killed.
func (r *Reaper) stopProcess(startTime time.Time) bool {
	// Step 2: Wait for Claude to be ready (prompt detection)
	if remaining := r.timeRemaining(startTime); remaining > 0 {
		r.logger.Info("Waiting for Claude to return to prompt")
		promptTimeout := min(PromptDetectionTimeout, remaining)
		if err := r.waitForPrompt(promptTimeout); err != nil {
			r.logger.Warn("Prompt detection failed", "error", err)
			fallback := min(FallbackWaitTime, r.timeRemaining(startTime))
			if fallback > 0 {
				r.logger.Info("Falling back to timer", "duration", fallback)
				time.Sleep(fallback)
			}
		} else {
			r.logger.Info("Prompt detected - Claude is ready")
		}
	}

	// Check for zombie timeout
	if r.timeRemaining(startTime) <= 0 {
		r.logger.Warn("Reaper timeout exceeded, force-archiving (skipping /exit)",
			"elapsed", time.Since(startTime), "timeout", ReaperTimeout)
		return true
	}

	// Step 3: Send /exit to exit Claude
	paneAlive := true
	r.logger.Info("Sending /exit to exit Claude")
	if err := r.sendExit(); err != nil {
		r.logger.Warn("Failed to send /exit (session may have already exited)", "error", err)
		if active, _ := tmux.IsPaneActive(r.SessionName); !active {
			paneAlive = false
			r.logger.Info("Pane already closed")
		}
	} else {
		r.logger.Info("/exit sent successfully")
	}

	// Step 4: Wait for pane to close
	if !paneAlive {
		return false
	}
	remaining := r.timeRemaining(startTime)
	if remaining <= 0 {
		r.logger.Warn("Reaper timeout exceeded after /exit, force-archiving",
			"elapsed", time.Since(startTime), "timeout", ReaperTimeout)
		return true
	}

	paneTimeout := min(PaneCloseTimeout, remaining)
	r.logger.Info("Waiting for pane to close", "timeout", paneTimeout)
	if err := r.waitForPaneClose(paneTimeout); err != nil {
		r.logger.Warn("Pane did not close after /exit, escalating to SIGTERM", "error", err)
		// Step 5: SIGTERM → wait → SIGKILL escalation
		r.forceKillPaneProcess()
	} else {
		r.logger.Info("Pane closed successfully after /exit")
	}

	return false
}

// timeRemaining returns the time remaining before ReaperTimeout is exceeded.
// Returns <= 0 if the timeout has been exceeded.
func (r *Reaper) timeRemaining(startTime time.Time) time.Duration {
	return ReaperTimeout - time.Since(startTime)
}

// forceKillPaneProcess escalates from SIGTERM to SIGKILL to ensure the
// pane process is dead. Called when /exit + wait times out.
func (r *Reaper) forceKillPaneProcess() {
	// Get the PID of the process in the pane
	pid, err := tmux.GetPanePID(r.SessionName)
	if err != nil {
		r.logger.Warn("Could not get pane PID (pane may already be gone)", "error", err)
		return
	}

	r.logger.Info("Sending SIGTERM to pane process", "pid", pid)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		r.logger.Warn("SIGTERM failed (process may already be gone)", "pid", pid, "error", err)
		return
	}

	// Wait for process to exit after SIGTERM
	r.logger.Info("Waiting for process to exit after SIGTERM", "grace_period", SIGTERMGracePeriod)
	if err := r.waitForPaneClose(SIGTERMGracePeriod); err == nil {
		r.logger.Info("Process exited after SIGTERM")
		return
	}

	// Escalate to SIGKILL
	r.logger.Warn("Process did not exit after SIGTERM, sending SIGKILL", "pid", pid)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		r.logger.Warn("SIGKILL failed", "pid", pid, "error", err)
	}

	// Brief wait for pane to close after SIGKILL
	if err := r.waitForPaneClose(PostKillPaneTimeout); err != nil {
		r.logger.Warn("Pane still alive after SIGKILL — will attempt tmux kill-session", "error", err)
	} else {
		r.logger.Info("Process killed with SIGKILL")
	}
}

// markReaping updates the session lifecycle to "reaping" in Dolt.
// This must happen BEFORE killing tmux so that if the reaper crashes,
// the GC startup scan can detect and archive the orphaned session.
func (r *Reaper) markReaping() error {
	sessionsDir, err := r.getSessionsDir()
	if err != nil {
		return fmt.Errorf("failed to get sessions directory: %w", err)
	}

	config, err := dolt.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to get Dolt config: %w", err)
	}

	adapter, err := dolt.New(config)
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt: %w", err)
	}
	defer adapter.Close()

	if err := adapter.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply Dolt migrations: %w", err)
	}

	m, _, err := session.ResolveIdentifier(r.SessionName, sessionsDir, adapter)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if m.Lifecycle == manifest.LifecycleArchived {
		return nil // already archived, nothing to do
	}

	m.Lifecycle = manifest.LifecycleReaping
	if err := adapter.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session in Dolt: %w", err)
	}

	r.logger.Info("Session marked as reaping in Dolt")
	return nil
}

// waitForPrompt monitors output stream for Claude prompt
// Uses tmux control mode to detect when Claude is ready for input
func (r *Reaper) waitForPrompt(timeout time.Duration) error {
	return tmux.WaitForPromptSimple(r.SessionName, timeout)
}

// sendExit sends /exit command to exit Claude Code cleanly
// Uses tmux.SendMultiLinePromptSafe which waits for prompt and sends literal /exit
// This avoids the sender attribution header that agm send adds
func (r *Reaper) sendExit() error {
	// Use tmux.SendMultiLinePromptSafe to send /exit as a literal command
	// This function:
	// 1. Waits for Claude prompt (handles busy pane states)
	// 2. Sends ESC to interrupt any thinking (shouldInterrupt=true)
	// 3. Sends /exit in literal mode (not as a message)
	// 4. Sends Enter to execute
	if err := tmux.SendMultiLinePromptSafe(r.SessionName, "/exit", true); err != nil {
		return fmt.Errorf("failed to send /exit: %w", err)
	}

	r.logger.Info("/exit sent via SendMultiLinePromptSafe")
	return nil
}

// waitForPaneClose waits for tmux pane to close
func (r *Reaper) waitForPaneClose(timeout time.Duration) error {
	return tmux.WaitForPaneClose(r.SessionName, timeout)
}

// archiveSession updates manifest and moves directory
// This is based on cmd/agm/archive.go but without interactive prompts
func (r *Reaper) archiveSession() error {
	sessionsDir, err := r.getSessionsDir()
	if err != nil {
		return fmt.Errorf("failed to get sessions directory: %w", err)
	}

	// Connect to Dolt database (needed for session resolution)
	config, err := dolt.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to get Dolt config: %w", err)
	}

	adapter, err := dolt.New(config)
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt: %w", err)
	}
	defer adapter.Close()

	// Apply migrations
	if err := adapter.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply Dolt migrations: %w", err)
	}

	// Resolve session identifier to manifest using Dolt adapter
	m, manifestPath, err := session.ResolveIdentifier(r.SessionName, sessionsDir, adapter)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	r.logger.Info("Manifest path", "path", manifestPath)

	// Check if already archived
	if m.Lifecycle == manifest.LifecycleArchived {
		r.logger.Info("Session already archived, skipping")
		return nil
	}

	// Best-effort MCP process cleanup before archiving
	sandboxPath := ""
	if m.Sandbox != nil {
		sandboxPath = m.Sandbox.MergedPath
	}
	mcpKilled, mcpErr := mcp.CleanupSessionMCPProcesses(
		&mcp.ProcFSFinder{}, &mcp.SignalKiller{},
		m.SessionID, sandboxPath,
	)
	if mcpErr != nil {
		r.logger.Warn("MCP cleanup error during reap", "session", r.SessionName, "error", mcpErr)
	}
	if mcpKilled > 0 {
		r.logger.Info("Cleaned up MCP processes during reap", "session", r.SessionName, "killed", mcpKilled)
	}

	// Update lifecycle field
	m.Lifecycle = manifest.LifecycleArchived

	// Update session in Dolt
	if err := adapter.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session in Dolt: %w", err)
	}
	r.logger.Info("Dolt database updated to archived")

	// Best-effort session resource cleanup (worktrees, branches, /tmp files)
	store := &cleanup.DoltWorktreeStore{Adapter: adapter}
	cleanupResult := cleanup.SessionResources(context.Background(), r.SessionName, store, cleanup.RealGitOps{}, r.logger)
	if cleanupResult.WorktreesRemoved > 0 {
		r.logger.Info("Cleaned up worktrees during reap", "count", cleanupResult.WorktreesRemoved)
	}
	if cleanupResult.BranchesDeleted > 0 {
		r.logger.Info("Deleted branches during reap", "count", cleanupResult.BranchesDeleted)
	}
	if cleanupResult.TmpFilesRemoved > 0 {
		r.logger.Info("Removed tmp files during reap", "count", cleanupResult.TmpFilesRemoved)
	}

	// Best-effort: clean up pending message directory for this session
	homeDir, err := os.UserHomeDir()
	if err == nil {
		pendingDir := filepath.Join(homeDir, ".agm", "pending", r.SessionName)
		if _, statErr := os.Stat(pendingDir); statErr == nil {
			if removeErr := os.RemoveAll(pendingDir); removeErr != nil {
				r.logger.Warn("Failed to remove pending directory", "path", pendingDir, "error", removeErr)
			} else {
				r.logger.Info("Cleaned up pending message directory", "path", pendingDir)
			}
		}
	}

	// Best-effort: move legacy session directory to .archive-old-format/ if it exists.
	// Sessions are now pure-Dolt, so filesystem directories may not exist.
	sessionDir := filepath.Dir(manifestPath)
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		r.logger.Info("No filesystem directory to move (pure-Dolt session)", "path", sessionDir)
	} else if err != nil {
		r.logger.Warn("Could not stat session directory, skipping move", "path", sessionDir, "error", err)
	} else {
		archiveBaseDir := filepath.Join(sessionsDir, ".archive-old-format")
		archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))

		// Create archive directory
		if err := os.MkdirAll(archiveBaseDir, 0700); err != nil {
			r.logger.Warn("Failed to create archive dir, skipping move", "error", err)
		} else {
			// Handle conflicts with timestamp
			if _, err := os.Stat(archiveTargetDir); err == nil {
				timestamp := time.Now().Format("20060102T150405Z")
				archiveTargetDir = archiveTargetDir + "-" + timestamp
				r.logger.Warn("Archive conflict - renaming", "target", filepath.Base(archiveTargetDir))
			}

			// Move directory (best-effort)
			if err := os.Rename(sessionDir, archiveTargetDir); err != nil {
				r.logger.Warn("Failed to move session directory to archive", "error", err)
			} else {
				r.logger.Info("Session moved to archive", "path", archiveTargetDir)
			}
		}
	}

	return nil
}

// getSessionsDir returns the configured sessions directory path
// Falls back to ~/sessions if not configured
func (r *Reaper) getSessionsDir() (string, error) {
	// Use configured directory if provided
	if r.SessionsDir != "" {
		return r.SessionsDir, nil
	}

	// Fall back to default ~/.claude/sessions
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".claude", "sessions"), nil
}
