package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectOrphanedDirectories(t *testing.T) {
	// Create temporary sandbox base directory
	tmpDir := t.TempDir()

	// Create some test directories with different ages
	oldDir := filepath.Join(tmpDir, "old-sandbox")
	newDir := filepath.Join(tmpDir, "new-sandbox")

	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("Failed to create new directory: %v", err)
	}

	// Make old directory appear old by changing its mtime
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to change mtime: %v", err)
	}

	// Detect orphaned directories (older than 1 hour)
	orphaned, err := DetectOrphanedDirectories(tmpDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("DetectOrphanedDirectories failed: %v", err)
	}

	// Should find exactly one orphaned directory
	if len(orphaned) != 1 {
		t.Errorf("Expected 1 orphaned directory, got %d", len(orphaned))
	}

	if len(orphaned) > 0 {
		if orphaned[0].Path != oldDir {
			t.Errorf("Expected orphaned directory %s, got %s", oldDir, orphaned[0].Path)
		}
		if orphaned[0].Type != "directory" {
			t.Errorf("Expected type 'directory', got '%s'", orphaned[0].Type)
		}
	}
}

func TestDetectOrphanedDirectories_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty directory should return no orphans
	orphaned, err := DetectOrphanedDirectories(tmpDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("DetectOrphanedDirectories failed: %v", err)
	}

	if len(orphaned) != 0 {
		t.Errorf("Expected 0 orphaned directories, got %d", len(orphaned))
	}
}

func TestDetectOrphanedDirectories_NonExistent(t *testing.T) {
	// Non-existent directory should return no error, no orphans
	orphaned, err := DetectOrphanedDirectories("/nonexistent/path", 1*time.Hour)
	if err != nil {
		t.Fatalf("Expected no error for non-existent directory, got: %v", err)
	}

	if orphaned != nil {
		t.Errorf("Expected nil orphans for non-existent directory, got %d", len(orphaned))
	}
}

func TestCleanupOrphanedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directories
	dir1 := filepath.Join(tmpDir, "sandbox-1")
	dir2 := filepath.Join(tmpDir, "sandbox-2")

	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatalf("Failed to create dir1: %v", err)
	}
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatalf("Failed to create dir2: %v", err)
	}

	// Create some files to track size
	testFile := filepath.Join(dir1, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create orphaned resources
	orphaned := []OrphanedResource{
		{
			Path: dir1,
			Type: "directory",
			Size: int64(len(content)),
		},
		{
			Path: dir2,
			Type: "directory",
			Size: 0,
		},
	}

	// Cleanup
	stats := CleanupOrphanedDirectories(orphaned)

	// Verify stats
	if stats.DirsDetected != 2 {
		t.Errorf("Expected 2 dirs detected, got %d", stats.DirsDetected)
	}
	if stats.DirsCleanedUp != 2 {
		t.Errorf("Expected 2 dirs cleaned up, got %d", stats.DirsCleanedUp)
	}
	if len(stats.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", stats.Errors)
	}

	// Verify directories are removed
	if _, err := os.Stat(dir1); !os.IsNotExist(err) {
		t.Errorf("Directory should be removed: %s", dir1)
	}
	if _, err := os.Stat(dir2); !os.IsNotExist(err) {
		t.Errorf("Directory should be removed: %s", dir2)
	}
}

func TestCleanupIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "sandbox-1")

	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatalf("Failed to create dir1: %v", err)
	}

	orphaned := []OrphanedResource{
		{Path: dir1, Type: "directory"},
	}

	// First cleanup
	stats1 := CleanupOrphanedDirectories(orphaned)
	if stats1.DirsCleanedUp != 1 {
		t.Errorf("First cleanup should succeed, got %d dirs cleaned", stats1.DirsCleanedUp)
	}

	// Second cleanup (should be idempotent)
	stats2 := CleanupOrphanedDirectories(orphaned)
	// Should still succeed even though directory doesn't exist
	if len(stats2.Errors) != 0 {
		t.Errorf("Second cleanup should be idempotent, got errors: %v", stats2.Errors)
	}
}

func TestRemoveWithRetry(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test")

	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Remove should succeed
	err := removeWithRetry(testDir)
	if err != nil {
		t.Errorf("removeWithRetry failed: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Errorf("Directory should be removed")
	}

	// Remove again should be idempotent
	err = removeWithRetry(testDir)
	if err != nil {
		t.Errorf("removeWithRetry should be idempotent, got error: %v", err)
	}
}

func TestCalculateDirSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files of known sizes
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	content1 := []byte("hello")  // 5 bytes
	content2 := []byte("world!") // 6 bytes

	if err := os.WriteFile(file1, content1, 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, content2, 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	size := calculateDirSize(tmpDir)
	expectedSize := int64(len(content1) + len(content2))

	if size != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, size)
	}
}

func TestCalculateDirSize_Nested(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(subDir, "file2.txt")

	content1 := []byte("test1") // 5 bytes
	content2 := []byte("test2") // 5 bytes

	if err := os.WriteFile(file1, content1, 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, content2, 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	size := calculateDirSize(tmpDir)
	expectedSize := int64(len(content1) + len(content2))

	if size != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, size)
	}
}

func TestIsNotMountedError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "not mounted error",
			err:      os.ErrInvalid,
			expected: false,
		},
		{
			name:     "EINVAL string",
			err:      &os.PathError{Op: "unmount", Path: "/test", Err: os.ErrInvalid},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotMountedError(tt.err)
			if result != tt.expected {
				t.Errorf("isNotMountedError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestCleanupStats_Tracking verifies that cleanup stats are tracked correctly.
func TestCleanupStats_Tracking(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple directories
	dirs := []string{
		filepath.Join(tmpDir, "sandbox-1"),
		filepath.Join(tmpDir, "sandbox-2"),
		filepath.Join(tmpDir, "sandbox-3"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
		// Add a file to track size
		testFile := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Create orphaned resources
	orphaned := []OrphanedResource{
		{Path: dirs[0], Type: "directory", Size: 100},
		{Path: dirs[1], Type: "directory", Size: 200},
		{Path: dirs[2], Type: "directory", Size: 300},
	}

	// Cleanup
	stats := CleanupOrphanedDirectories(orphaned)

	// Verify all were cleaned
	if stats.DirsDetected != 3 {
		t.Errorf("Expected 3 dirs detected, got %d", stats.DirsDetected)
	}
	if stats.DirsCleanedUp != 3 {
		t.Errorf("Expected 3 dirs cleaned up, got %d", stats.DirsCleanedUp)
	}
	if stats.TotalBytesFreed != 600 {
		t.Errorf("Expected 600 bytes freed, got %d", stats.TotalBytesFreed)
	}
}
