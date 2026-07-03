package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAcquireLock_FreshAndConcurrent verifies a fresh lock is acquired and a
// second acquisition against a live-PID lock is rejected.
func TestAcquireLock_FreshAndConcurrent(t *testing.T) {
	dir := t.TempDir()
	lockDir := filepath.Join(dir, "locks")
	lockPath := filepath.Join(lockDir, "job.lock")

	if err := acquireLock(lockDir, lockPath, "job"); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	// The lock records our own (live) PID, so a second acquire must fail.
	if err := acquireLock(lockDir, lockPath, "job"); err == nil {
		t.Fatal("second acquire against live lock succeeded, want failure")
	}

	releaseLock(lockPath)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock dir still present after release: %v", err)
	}
}

// TestAcquireLock_StaleRecovery is the regression test for the bug where a
// stale lock dir containing a "pid" file could never be reclaimed because
// os.Remove refuses non-empty directories. A dead PID must be recovered.
func TestAcquireLock_StaleRecovery(t *testing.T) {
	dir := t.TempDir()
	lockDir := filepath.Join(dir, "locks")
	lockPath := filepath.Join(lockDir, "job.lock")

	if err := os.MkdirAll(lockPath, 0o700); err != nil {
		t.Fatalf("seed lock dir: %v", err)
	}
	// PID 2^30 is far above any real PID and is guaranteed dead — marks the
	// lock as stale so acquireLock must clear it (dir + pid file) and re-lock.
	if err := os.WriteFile(filepath.Join(lockPath, "pid"), []byte("1073741824\n"), 0o600); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	if err := acquireLock(lockDir, lockPath, "job"); err != nil {
		t.Fatalf("stale lock not recovered: %v", err)
	}
	// After recovery the lock must belong to us: pid file rewritten with our PID.
	pidBytes, err := os.ReadFile(filepath.Join(lockPath, "pid"))
	if err != nil {
		t.Fatalf("read pid after recovery: %v", err)
	}
	if got := string(pidBytes); got == "1073741824\n" {
		t.Fatalf("pid file still holds stale pid %q, lock not re-taken", got)
	}
}
