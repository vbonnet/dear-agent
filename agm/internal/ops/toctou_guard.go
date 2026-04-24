package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// lockDir returns the directory where session lock files are stored.
func lockDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".agm", "locks")
}

// ErrCodeLockTimeout is the error code returned when a session lock
// cannot be acquired within the timeout period.
const ErrCodeLockTimeout = "AGM-014"

// ErrLockTimeout returns an OpError for lock acquisition timeout.
func ErrLockTimeout(sessionName string, timeout time.Duration) *OpError {
	return &OpError{
		Status: 409,
		Type:   "session/lock_timeout",
		Code:   ErrCodeLockTimeout,
		Title:  "Session lock timeout",
		Detail: fmt.Sprintf("Could not acquire lock for session %q within %s. Another agm command may be operating on this session.", sessionName, timeout),
		Suggestions: []string{
			"Wait for the other operation to complete and retry.",
			fmt.Sprintf("Remove stale lock manually: rm ~/.agm/locks/%s.lock", sessionName),
			"Run `agm admin doctor` to check for stale locks.",
		},
		Parameters: map[string]string{
			"session_name": sessionName,
			"timeout":      timeout.String(),
		},
	}
}

// WithSessionLock acquires an exclusive file lock for the named session,
// executes fn, then releases the lock. This prevents TOCTOU races between
// parallel agm commands operating on the same session.
//
// The lock file is created at ~/.agm/locks/{sessionName}.lock.
// If the lock cannot be acquired within the configured timeout, ErrLockTimeout is returned.
func WithSessionLock(sessionName string, fn func() error) error {
	slo := contracts.Load()
	return WithSessionLockTimeout(sessionName, slo.SessionLifecycle.LockTimeout.Duration, fn)
}

// WithSessionLockTimeout is like WithSessionLock but accepts a custom timeout.
func WithSessionLockTimeout(sessionName string, timeout time.Duration, fn func() error) error {
	slo := contracts.Load()
	pollInterval := slo.SessionLifecycle.LockPollInterval.Duration

	dir := lockDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}

	lockPath := filepath.Join(dir, sessionName+".lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer f.Close()

	// Try non-blocking lock in a polling loop up to timeout.
	deadline := time.Now().Add(timeout)
	fd := int(f.Fd()) //nolint:gosec // Fd() returns a valid file descriptor; overflow not possible.

	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return ErrLockTimeout(sessionName, timeout)
		}
		time.Sleep(pollInterval)
	}

	// Lock acquired — ensure unlock on exit.
	defer func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
	}()

	return fn()
}
