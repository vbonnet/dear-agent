package tmux

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/lock"
)

// tmuxServerLock protects tmux server state mutations from parallel access.
// This prevents race conditions when multiple CSM commands update tmux settings,
// send commands, or modify session state concurrently.
//
// Lock scope: Only tmux server mutations (NewSession settings, SendCommand, InitSequence)
// NOT locked: Read operations (HasSession, ListSessions) and AttachSession (can block indefinitely)
var tmuxServerLock *lock.FileLock

// getStateDir returns the CSM state directory.
// Uses CSM_STATE_DIR environment variable if set (for test isolation),
// otherwise defaults to /tmp/csm-{UID} (production default).
func getStateDir() string {
	stateDir := os.Getenv("CSM_STATE_DIR")
	if stateDir == "" {
		uid := os.Getuid()
		stateDir = fmt.Sprintf("/tmp/csm-%d", uid)
	}
	return stateDir
}

// AcquireTmuxLock locks tmux server mutations to prevent parallel updates.
// This is a fine-grained lock (only tmux operations, not entire CSM commands).
//
// Lock path: $CSM_STATE_DIR/tmux-server.lock (defaults to /tmp/csm-{UID}/tmux-server.lock)
// Set CSM_STATE_DIR environment variable for test isolation.
//
// Returns error if lock is already held by another process.
func AcquireTmuxLock() error {
	if tmuxServerLock != nil {
		// Already locked by this process - this is a bug
		return fmt.Errorf("tmux lock already held by this process (double lock)")
	}

	stateDir := getStateDir()
	lockPath := filepath.Join(stateDir, "tmux-server.lock")

	var err error
	tmuxServerLock, err = lock.New(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create tmux lock: %w", err)
	}

	if err := tmuxServerLock.TryLock(); err != nil {
		tmuxServerLock = nil // Reset on failure
		return err
	}

	return nil
}

// ReleaseTmuxLock releases the tmux server lock.
// Safe to call multiple times (subsequent calls are no-ops).
func ReleaseTmuxLock() error {
	if tmuxServerLock == nil {
		return nil
	}
	err := tmuxServerLock.Unlock()
	tmuxServerLock = nil // Clear after unlock
	return err
}
