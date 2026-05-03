// Package lock provides lock functionality.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileLock represents a file-based lock using syscall.Flock
type FileLock struct {
	file *os.File
	path string
}

// LockError represents a lock acquisition error with recovery guidance
type LockError struct {
	Problem  string
	Recovery string
}

func (e *LockError) Error() string {
	msg := fmt.Sprintf("Error: %s\n\n", e.Problem)
	if e.Recovery != "" {
		msg += fmt.Sprintf("%s\n", e.Recovery)
	}
	return msg
}

// LockInfo contains information about the current lock state
type LockInfo struct {
	Exists    bool
	PID       int
	IsStale   bool // Process not running
	CanUnlock bool // Safe to remove
	Path      string
}

// New creates a new file lock at the specified path.
// Creates the lock file and its parent directory if they don't exist.
func New(path string) (*FileLock, error) {
	// Create lock directory if missing
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Open lock file (create if missing)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	return &FileLock{file: file, path: path}, nil
}

// TryLock attempts to acquire the lock (non-blocking).
// Returns LockError if lock is already held by another process.
func (fl *FileLock) TryLock() error {
	// LOCK_EX = exclusive lock
	// LOCK_NB = non-blocking (fail immediately if locked)
	err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return &LockError{
			Problem:  "Another agm command is currently running",
			Recovery: "Wait for the other command to finish, or run 'agm unlock' to check for stale locks",
		}
	}

	// Write PID to lock file for debugging
	pid := fmt.Sprintf("%d\n", os.Getpid())
	fl.file.Truncate(0)
	fl.file.Seek(0, 0)
	fl.file.WriteString(pid)

	return nil
}

// Unlock releases the lock and closes the file.
// Safe to call multiple times (subsequent calls are no-ops).
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}
	syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN)
	err := fl.file.Close()
	fl.file = nil // Prevent double-close
	return err
}

// DefaultLockPath returns the default lock file path.
// If AGM_LOCK_PATH is set (test sandbox isolation), it is used instead.
// Otherwise: /tmp/agm-{UID}/agm.lock
func DefaultLockPath() (string, error) {
	if lockPath := os.Getenv("AGM_LOCK_PATH"); lockPath != "" {
		return lockPath, nil
	}
	uid := os.Getuid()
	return fmt.Sprintf("/tmp/agm-%d/agm.lock", uid), nil
}

// CheckLock checks the status of a lock file
func CheckLock(path string) (*LockInfo, error) {
	info := &LockInfo{
		Path:   path,
		Exists: false,
	}

	// Check if lock file exists
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No lock file - safe to proceed
			info.CanUnlock = false
			return info, nil
		}
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	info.Exists = true

	// Parse PID from lock file
	pidStr := string(data)
	if pidStr == "" {
		// Empty lock file - stale
		info.IsStale = true
		info.CanUnlock = true
		return info, nil
	}

	var pid int
	_, err = fmt.Sscanf(pidStr, "%d", &pid)
	if err != nil {
		// Invalid PID - stale
		info.IsStale = true
		info.CanUnlock = true
		return info, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	info.PID = pid

	// Check if process is still running
	if !processExists(pid) {
		info.IsStale = true
		info.CanUnlock = true
		return info, nil
	}

	// Process is running - lock is active
	info.IsStale = false
	info.CanUnlock = false
	return info, nil
}

// processExists checks if a process with the given PID is running
func processExists(pid int) bool {
	// Try Linux /proc method first (most reliable)
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
		return true
	}

	// Fallback: try to send signal 0 (null signal - just checks if process exists)
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 doesn't actually send a signal, just checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ForceUnlock removes a lock file without checking if the process is running
// This should only be used when the user explicitly requests it with --force
func ForceUnlock(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}
	return nil
}
