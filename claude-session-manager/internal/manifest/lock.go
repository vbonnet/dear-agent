package manifest

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// AcquireLock attempts to acquire an exclusive lock on a manifest file
// Lock file format: <PID>\n<RFC3339>\n
// Returns error if lock already held or if lock file cannot be created
func AcquireLock(manifestPath string) error {
	lockPath := manifestPath + ".lock"

	// Check if lock already exists
	if data, err := os.ReadFile(lockPath); err == nil {
		// Lock file exists, check if stale
		lines := strings.Split(string(data), "\n")
		if len(lines) >= 2 {
			// Parse timestamp
			if lockTime, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1])); err == nil {
				age := time.Since(lockTime)
				if age < LockTimeout {
					// Lock is fresh, return error with details
					pid := strings.TrimSpace(lines[0])
					return fmt.Errorf("session is locked by process %s (started %s)\n\nTry one of the following:\n  • Wait a minute and retry (process may finish)\n  • Check if process is still running: ps -p %s\n  • If process is stuck, kill it: kill %s\n  • Check for stale locks: csm doctor --fix",
						pid, lockTime.Format(time.RFC3339), pid, pid)
				}
				// Lock is stale (>60s old), will overwrite
			}
		}
	}

	// Create lock file
	pid := os.Getpid()
	lockTime := time.Now()
	lockContent := fmt.Sprintf("%d\n%s\n", pid, lockTime.Format(time.RFC3339))

	if err := os.WriteFile(lockPath, []byte(lockContent), 0600); err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	return nil
}

// ReleaseLock releases the lock on a manifest file
// Returns error if lock file cannot be removed
func ReleaseLock(manifestPath string) error {
	lockPath := manifestPath + ".lock"

	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}

	return nil
}

// IsLocked checks if a manifest file is currently locked
// Returns true if lock exists and is fresh (< 60s old)
func IsLocked(manifestPath string) bool {
	lockPath := manifestPath + ".lock"

	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false // Lock file doesn't exist
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 {
		return false // Malformed lock file
	}

	// Parse timestamp
	lockTime, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1]))
	if err != nil {
		return false // Invalid timestamp
	}

	age := time.Since(lockTime)
	return age < LockTimeout
}

// GetLockInfo returns information about the current lock, if any
// Returns PID, timestamp, and age of the lock
func GetLockInfo(manifestPath string) (pid int, lockTime time.Time, err error) {
	lockPath := manifestPath + ".lock"

	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, time.Time{}, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 {
		return 0, time.Time{}, fmt.Errorf("malformed lock file")
	}

	// Parse PID
	pidStr := strings.TrimSpace(lines[0])
	pid, err = strconv.Atoi(pidStr)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("invalid PID in lock file: %w", err)
	}

	// Parse timestamp
	lockTime, err = time.Parse(time.RFC3339, strings.TrimSpace(lines[1]))
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("invalid timestamp in lock file: %w", err)
	}

	return pid, lockTime, nil
}
