package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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

	if err := fl.Lock(); err != nil {
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

// withTmuxLockContext waits for the tmux mutation lock only while ctx remains
// live. It is used by capture-to-input authority transactions, where an
// unbounded flock would violate the caller's readiness deadline.
func withTmuxLockContext(ctx context.Context, fn func() error) (resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := acquireTmuxLockContext(ctx); err != nil {
		return fmt.Errorf("failed to acquire tmux lock: %w", err)
	}
	defer func() {
		if err := ReleaseTmuxLock(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("release tmux lock: %w", err)
		}
	}()
	return fn()
}

func acquireTmuxLockContext(ctx context.Context) error {
	const retryInterval = 10 * time.Millisecond
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		acquired, err := tryAcquireTmuxLock()
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func tryAcquireTmuxLock() (bool, error) {
	if !tmuxServerLockMu.TryLock() {
		return false, nil
	}
	defer tmuxServerLockMu.Unlock()
	if tmuxServerLock != nil {
		return false, nil
	}
	lockPath := filepath.Join(getStateDir(), "tmux-server.lock")
	fileLock, err := lock.New(lockPath)
	if err != nil {
		return false, fmt.Errorf("failed to create tmux lock: %w", err)
	}
	if err := fileLock.TryLock(); err != nil {
		_ = fileLock.Unlock()
		var contention *lock.LockError
		if errors.As(err, &contention) {
			return false, nil
		}
		return false, err
	}
	tmuxServerLock = fileLock
	return true, nil
}
