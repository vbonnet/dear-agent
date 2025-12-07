package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Create dummy manifest file
	if err := os.WriteFile(manifestPath, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	// Acquire lock
	if err := AcquireLock(manifestPath); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock file exists
	lockPath := manifestPath + ".lock"
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file was not created")
	}

	// Release lock
	if err := ReleaseLock(manifestPath); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Verify lock file removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("Lock file was not removed")
	}
}

func TestAcquireLock_AlreadyLocked(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Acquire first lock
	if err := AcquireLock(manifestPath); err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	defer ReleaseLock(manifestPath)

	// Try to acquire second lock (should fail)
	err := AcquireLock(manifestPath)
	if err == nil {
		t.Error("Expected error when acquiring lock twice, got nil")
	}

	// Error message should contain PID
	if err != nil && len(err.Error()) < 20 {
		t.Errorf("Error message too short: %v", err)
	}
}

func TestIsLocked(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Initially not locked
	if IsLocked(manifestPath) {
		t.Error("Expected IsLocked to return false initially")
	}

	// Acquire lock
	if err := AcquireLock(manifestPath); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Now locked
	if !IsLocked(manifestPath) {
		t.Error("Expected IsLocked to return true after acquiring lock")
	}

	// Release lock
	if err := ReleaseLock(manifestPath); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Not locked again
	if IsLocked(manifestPath) {
		t.Error("Expected IsLocked to return false after releasing lock")
	}
}

func TestGetLockInfo(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Acquire lock
	beforeLock := time.Now()
	if err := AcquireLock(manifestPath); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer ReleaseLock(manifestPath)
	afterLock := time.Now()

	// Get lock info
	pid, lockTime, err := GetLockInfo(manifestPath)
	if err != nil {
		t.Fatalf("Failed to get lock info: %v", err)
	}

	// Verify PID
	expectedPID := os.Getpid()
	if pid != expectedPID {
		t.Errorf("Wrong PID: got %d, want %d", pid, expectedPID)
	}

	// Verify timestamp is recent
	if lockTime.Before(beforeLock) || lockTime.After(afterLock) {
		t.Errorf("Lock timestamp out of range: %v (expected between %v and %v)",
			lockTime, beforeLock, afterLock)
	}
}

func TestLockTimeout_Stale(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	lockPath := manifestPath + ".lock"

	// Create stale lock (2 minutes old)
	stalePID := 99999
	staleTime := time.Now().Add(-2 * time.Minute)
	staleLock := []byte(staleTime.Format("99999\n2006-01-02T15:04:05Z07:00\n"))
	if err := os.WriteFile(lockPath, staleLock, 0600); err != nil {
		t.Fatalf("Failed to create stale lock: %v", err)
	}

	// Should not be considered locked (stale)
	if IsLocked(manifestPath) {
		t.Error("Stale lock should not be considered locked")
	}

	// Should be able to acquire lock (overwrites stale lock)
	if err := AcquireLock(manifestPath); err != nil {
		t.Errorf("Should be able to acquire lock over stale lock: %v", err)
	}
	defer ReleaseLock(manifestPath)

	// Verify new lock has current PID
	pid, _, err := GetLockInfo(manifestPath)
	if err != nil {
		t.Fatalf("Failed to get lock info: %v", err)
	}
	if pid == stalePID {
		t.Error("Lock should have been replaced with new PID")
	}
	if pid != os.Getpid() {
		t.Errorf("Lock PID should be current process: got %d, want %d", pid, os.Getpid())
	}
}

func TestReleaseLock_NotLocked(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Releasing non-existent lock should not error
	if err := ReleaseLock(manifestPath); err != nil {
		t.Errorf("ReleaseLock on non-locked manifest should not error: %v", err)
	}
}
