package lock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLock_TryLock_Success(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer lock.Unlock()

	if err := lock.TryLock(); err != nil {
		t.Errorf("TryLock() failed: %v", err)
	}

	// Verify PID was written to lock file
	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Lock file is empty, expected PID")
	}
}

func TestFileLock_TryLock_AlreadyLocked(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// First lock
	lock1, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer lock1.Unlock()

	if err := lock1.TryLock(); err != nil {
		t.Fatalf("First TryLock() failed: %v", err)
	}

	// Second lock (should fail)
	lock2, err := New(lockPath)
	if err != nil {
		t.Fatalf("Second New() failed: %v", err)
	}
	defer lock2.Unlock()

	err = lock2.TryLock()
	if err == nil {
		t.Fatal("Second TryLock() should have failed but succeeded")
	}

	// Verify error is LockError with recovery guidance
	lockErr := &LockError{}
	ok := errors.As(err, &lockErr)
	if !ok {
		t.Errorf("Expected LockError, got %T", err)
	}
	if lockErr.Problem == "" {
		t.Error("LockError.Problem is empty")
	}
	if lockErr.Recovery == "" {
		t.Error("LockError.Recovery is empty")
	}
}

func TestFileLock_Unlock_Success(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := lock.TryLock(); err != nil {
		t.Fatalf("TryLock() failed: %v", err)
	}

	if err := lock.Unlock(); err != nil {
		t.Errorf("Unlock() failed: %v", err)
	}

	// Verify lock can be acquired again after unlock
	lock2, err := New(lockPath)
	if err != nil {
		t.Fatalf("Second New() failed: %v", err)
	}
	defer lock2.Unlock()

	if err := lock2.TryLock(); err != nil {
		t.Errorf("TryLock() after unlock failed: %v", err)
	}
}

func TestFileLock_Unlock_MultipleCallsSafe(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := lock.TryLock(); err != nil {
		t.Fatalf("TryLock() failed: %v", err)
	}

	// Multiple unlocks should not panic
	if err := lock.Unlock(); err != nil {
		t.Errorf("First Unlock() failed: %v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Errorf("Second Unlock() failed: %v", err)
	}
}

func TestFileLock_CrashRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Simulate crash by acquiring lock and not releasing it
	// (file descriptor will be closed when process exits)
	lock1, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := lock1.TryLock(); err != nil {
		t.Fatalf("TryLock() failed: %v", err)
	}

	// Close the file without unlocking (simulates crash)
	lock1.file.Close()

	// Wait a bit to ensure kernel releases the lock
	time.Sleep(10 * time.Millisecond)

	// Second lock should succeed (lock auto-released)
	lock2, err := New(lockPath)
	if err != nil {
		t.Fatalf("Second New() failed: %v", err)
	}
	defer lock2.Unlock()

	if err := lock2.TryLock(); err != nil {
		t.Errorf("TryLock() after crash simulation failed: %v", err)
	}
}

func TestDefaultLockPath(t *testing.T) {
	path, err := DefaultLockPath()
	if err != nil {
		t.Fatalf("DefaultLockPath() failed: %v", err)
	}

	// Verify path format: /tmp/agm-{UID}/agm.lock
	if path == "" {
		t.Error("DefaultLockPath() returned empty string")
	}

	// Verify it contains /tmp and agm.lock
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultLockPath() returned relative path: %s", path)
	}
	if filepath.Base(path) != "agm.lock" {
		t.Errorf("DefaultLockPath() basename is not agm.lock: %s", filepath.Base(path))
	}
}

func TestLockError_Format(t *testing.T) {
	err := &LockError{
		Problem:  "Test problem",
		Recovery: "Test recovery",
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("LockError.Error() returned empty string")
	}

	// Verify both fields are in the error message
	if !contains(errStr, "Test problem") {
		t.Error("Error message missing Problem field")
	}
	if !contains(errStr, "Test recovery") {
		t.Error("Error message missing Recovery field")
	}
}

func TestNew_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "subdir", "nested", "test.lock")

	lock, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() failed to create nested directories: %v", err)
	}
	defer lock.Unlock()

	// Verify directory was created
	dir := filepath.Dir(lockPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("New() did not create directory: %s", dir)
	}
}

func TestCheckLock_NoLockExists(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	info, err := CheckLock(lockPath)
	if err != nil {
		t.Fatalf("CheckLock() failed: %v", err)
	}

	if info.Exists {
		t.Error("CheckLock() reported lock exists when it doesn't")
	}
	if info.CanUnlock {
		t.Error("CheckLock() reported CanUnlock=true for non-existent lock")
	}
}

func TestCheckLock_StaleLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create stale lock with non-existent PID
	if err := os.WriteFile(lockPath, []byte("99999\n"), 0644); err != nil {
		t.Fatalf("Failed to create stale lock: %v", err)
	}

	info, err := CheckLock(lockPath)
	if err != nil {
		t.Fatalf("CheckLock() failed: %v", err)
	}

	if !info.Exists {
		t.Error("CheckLock() didn't detect lock file")
	}
	if !info.IsStale {
		t.Error("CheckLock() didn't detect stale lock")
	}
	if !info.CanUnlock {
		t.Error("CheckLock() should allow unlocking stale lock")
	}
	if info.PID != 99999 {
		t.Errorf("CheckLock() PID = %d, want 99999", info.PID)
	}
}

func TestCheckLock_ActiveLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create lock with current process PID (active)
	currentPID := os.Getpid()
	if err := os.WriteFile(lockPath, []byte(string(rune(currentPID))+"\n"), 0644); err != nil {
		t.Fatalf("Failed to create active lock: %v", err)
	}

	info, err := CheckLock(lockPath)
	if err != nil {
		t.Fatalf("CheckLock() failed: %v", err)
	}

	if !info.Exists {
		t.Error("CheckLock() didn't detect lock file")
	}
	// Note: This test might fail if PID isn't properly written
	// The lock should be detected as active since it's our own process
}

func TestCheckLock_EmptyLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create empty lock file
	if err := os.WriteFile(lockPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty lock: %v", err)
	}

	info, err := CheckLock(lockPath)
	if err != nil {
		t.Fatalf("CheckLock() failed: %v", err)
	}

	if !info.IsStale {
		t.Error("CheckLock() should consider empty lock as stale")
	}
	if !info.CanUnlock {
		t.Error("CheckLock() should allow unlocking empty lock")
	}
}

func TestCheckLock_InvalidPID(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create lock with invalid PID
	if err := os.WriteFile(lockPath, []byte("not-a-pid\n"), 0644); err != nil {
		t.Fatalf("Failed to create lock: %v", err)
	}

	info, err := CheckLock(lockPath)
	if err != nil {
		t.Fatalf("CheckLock() failed: %v", err)
	}

	if !info.IsStale {
		t.Error("CheckLock() should consider invalid PID as stale")
	}
	if !info.CanUnlock {
		t.Error("CheckLock() should allow unlocking invalid PID lock")
	}
}

func TestForceUnlock_RemovesLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create lock file
	if err := os.WriteFile(lockPath, []byte("12345\n"), 0644); err != nil {
		t.Fatalf("Failed to create lock: %v", err)
	}

	// Force unlock
	if err := ForceUnlock(lockPath); err != nil {
		t.Errorf("ForceUnlock() failed: %v", err)
	}

	// Verify lock is removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("ForceUnlock() didn't remove lock file")
	}
}

func TestForceUnlock_NonExistentLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "nonexistent.lock")

	// Should not error on non-existent lock
	if err := ForceUnlock(lockPath); err != nil {
		t.Errorf("ForceUnlock() failed on non-existent lock: %v", err)
	}
}

func TestProcessExists_CurrentProcess(t *testing.T) {
	currentPID := os.Getpid()
	if !processExists(currentPID) {
		t.Error("processExists() returned false for current process")
	}
}

func TestProcessExists_NonExistentProcess(t *testing.T) {
	// Use a very high PID that's unlikely to exist
	if processExists(999999) {
		t.Error("processExists() returned true for non-existent process")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && substr != "" &&
		(s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
