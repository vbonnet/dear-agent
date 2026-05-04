package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
)

// tmuxServerLock protects tmux server state mutations from parallel access.
// This prevents race conditions when multiple AGM commands update tmux settings,
// send commands, or modify session state concurrently.
//
// Lock scope: Only tmux server mutations (NewSession settings, SendCommand, InitSequence)
// NOT locked: Read operations (HasSession, ListSessions) and AttachSession (can block indefinitely)
var (
	tmuxServerLock   *lock.FileLock
	tmuxServerLockMu sync.Mutex
)

// getStateDir returns the AGM state directory.
// Uses AGM_STATE_DIR environment variable if set (for test isolation),
// otherwise defaults to /tmp/agm-{UID} (production default).
func getStateDir() string {
	stateDir := os.Getenv("AGM_STATE_DIR")
	if stateDir == "" {
		uid := os.Getuid()
		stateDir = fmt.Sprintf("/tmp/agm-%d", uid)
	}
	return stateDir
}

// AcquireTmuxLock locks tmux server mutations to prevent parallel updates.
// This is a fine-grained lock (only tmux operations, not entire AGM commands).
//
// Lock path: $AGM_STATE_DIR/tmux-server.lock (defaults to /tmp/agm-{UID}/tmux-server.lock)
// Set AGM_STATE_DIR environment variable for test isolation.
//
// Returns error if lock is already held by another process.
func AcquireTmuxLock() error {
	tmuxServerLockMu.Lock()
	defer tmuxServerLockMu.Unlock()

	if tmuxServerLock != nil {
		// Already locked by this process - this is a bug
		return fmt.Errorf("tmux lock already held by this process (double lock)")
	}

	stateDir := getStateDir()
	lockPath := filepath.Join(stateDir, "tmux-server.lock")

	fl, err := lock.New(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create tmux lock: %w", err)
	}

	if err := fl.TryLock(); err != nil {
		return err
	}

	tmuxServerLock = fl
	return nil
}

// ReleaseTmuxLock releases the tmux server lock.
// Safe to call multiple times (subsequent calls are no-ops).
func ReleaseTmuxLock() error {
	tmuxServerLockMu.Lock()
	defer tmuxServerLockMu.Unlock()

	if tmuxServerLock == nil {
		return nil
	}
	err := tmuxServerLock.Unlock()
	tmuxServerLock = nil // Clear after unlock
	return err
}

// withTmuxLock executes the provided function while holding the tmux server lock.
// The lock is automatically acquired before fn executes and released after (even on panic).
//
// This helper consolidates the lock acquisition/release pattern used across tmux operations,
// ensuring consistent error handling and preventing lock leaks.
//
// Example:
//
//	return withTmuxLock(func() error {
//	    // ... tmux operation code ...
//	    return nil
//	})
func withTmuxLock(fn func() error) error {
	if err := AcquireTmuxLock(); err != nil {
		return fmt.Errorf("failed to acquire tmux lock: %w", err)
	}
	defer ReleaseTmuxLock()
	return fn()
}
