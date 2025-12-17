package lock

import (
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
	lockErr, ok := err.(*LockError)
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

	// Verify path format: /tmp/csm-{UID}/csm.lock
	if path == "" {
		t.Error("DefaultLockPath() returned empty string")
	}

	// Verify it contains /tmp and csm.lock
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultLockPath() returned relative path: %s", path)
	}
	if filepath.Base(path) != "csm.lock" {
		t.Errorf("DefaultLockPath() basename is not csm.lock: %s", filepath.Base(path))
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
