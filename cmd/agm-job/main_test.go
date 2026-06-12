package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAcquireReleaseLock verifies basic lock acquire / release semantics.
func TestAcquireReleaseLock(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "test-lock")
	log := &jobLogger{f: nil}

	// First acquire should succeed.
	acquired, err := acquireLock(lockDir, log)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	// Second acquire while held should return false (not an error).
	acquired2, err2 := acquireLock(lockDir, log)
	if err2 != nil {
		t.Fatalf("second acquireLock: %v", err2)
	}
	if acquired2 {
		t.Fatal("expected second acquire to fail (lock held)")
	}

	// Release.
	releaseLock(lockDir, log)
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Error("lock dir should not exist after release")
	}

	// Re-acquire after release should succeed.
	acquired3, err3 := acquireLock(lockDir, log)
	if err3 != nil {
		t.Fatalf("re-acquire: %v", err3)
	}
	if !acquired3 {
		t.Fatal("expected re-acquire after release to succeed")
	}
	releaseLock(lockDir, log)
}

// TestAcquireLock_StalePid verifies that a lock with a dead PID is reclaimed.
func TestAcquireLock_StalePid(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "stale-lock")

	// Create a lock dir with a dead PID (PID 1 is unlikely to be our process).
	if err := os.Mkdir(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock: %v", err)
	}
	// Write a PID that does not correspond to a process we own.
	_ = os.WriteFile(filepath.Join(lockDir, "pid"), []byte("99999999"), 0o600)

	log := &jobLogger{f: nil}
	acquired, err := acquireLock(lockDir, log)
	if err != nil {
		t.Fatalf("acquireLock with stale pid: %v", err)
	}
	if !acquired {
		t.Fatal("expected stale lock to be reclaimed")
	}
	releaseLock(lockDir, log)
}

// TestRotateLogs verifies that a file larger than maxLogBytes is rotated.
func TestRotateLogs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Write a file that exceeds maxLogBytes.
	big := make([]byte, maxLogBytes+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(logPath, big, 0o600); err != nil {
		t.Fatalf("write big log: %v", err)
	}

	rotateLogs(logPath)

	// Original should be gone; .1 rotation should exist.
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("original log should have been rotated away")
	}
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Errorf("rotated log .1 missing: %v", err)
	}
}

// TestJobLogger verifies the logger writes to a file without panicking.
func TestJobLogger(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "job.log")

	log := newJobLogger(logPath)
	log.Printf("test message %d", 42)
	log.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) == 0 {
		t.Error("log file is empty")
	}

	// Verify timestamp prefix is present.
	line := string(data)
	if len(line) < 20 || line[0] != '[' {
		t.Errorf("unexpected log format: %q", line)
	}
	_ = time.Now() // satisfy linter (unused import guard)
}
