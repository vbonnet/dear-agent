package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupOrphanedMounts_Empty(t *testing.T) {
	stats := CleanupOrphanedMounts(nil)
	assert.Equal(t, 0, stats.MountsDetected)
	assert.Equal(t, 0, stats.MountsCleanedUp)
	assert.Empty(t, stats.Errors)
}

func TestCleanupOrphanedMounts_SkipsNonMountTypes(t *testing.T) {
	resources := []OrphanedResource{
		{Path: "/fake/path", Type: "directory"},
	}
	stats := CleanupOrphanedMounts(resources)
	assert.Equal(t, 1, stats.MountsDetected)
	assert.Equal(t, 0, stats.MountsCleanedUp) // skipped because Type != "mount"
}

func TestCleanupOrphanedDirectories_SkipsNonDirTypes(t *testing.T) {
	resources := []OrphanedResource{
		{Path: "/fake/path", Type: "mount"},
	}
	stats := CleanupOrphanedDirectories(resources)
	assert.Equal(t, 1, stats.DirsDetected)
	assert.Equal(t, 0, stats.DirsCleanedUp) // skipped because Type != "directory"
}

func TestCleanupOrphaned_Integration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an old directory
	oldDir := filepath.Join(tmpDir, "old-sandbox")
	require.NoError(t, os.MkdirAll(oldDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "data.txt"), []byte("test"), 0644))

	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	// Run full cleanup
	stats, err := CleanupOrphaned(tmpDir, 1*time.Hour)
	require.NoError(t, err)

	// Should have detected the old directory
	assert.Equal(t, 1, stats.DirsDetected)
	assert.Equal(t, 1, stats.DirsCleanedUp)

	// Directory should be removed
	_, err = os.Stat(oldDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupOrphaned_NoOldDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a new directory (not old enough)
	newDir := filepath.Join(tmpDir, "new-sandbox")
	require.NoError(t, os.MkdirAll(newDir, 0755))

	stats, err := CleanupOrphaned(tmpDir, 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.DirsDetected)
}

func TestDetectOrphanedMounts_ReturnsNilOnNonLinux(t *testing.T) {
	// On Linux this reads /proc/mounts; verify it returns no error even with a valid base dir
	orphaned, err := DetectOrphanedMounts(t.TempDir())
	assert.NoError(t, err)
	// May or may not find orphaned mounts depending on system state
	_ = orphaned
}

func TestIsNotMountedError_MoreCases(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil_error",
			err:      nil,
			expected: false,
		},
		{
			name:     "not_mounted_string",
			err:      errors.New("device not mounted"),
			expected: true,
		},
		{
			name:     "not_a_mount_point",
			err:      errors.New("not a mount point"),
			expected: true,
		},
		{
			name:     "EINVAL_in_message",
			err:      errors.New("operation failed: EINVAL"),
			expected: true,
		},
		{
			name:     "unrelated_error",
			err:      errors.New("permission denied"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotMountedError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectOrphanedDirectories_SkipsFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file (not a directory) -- should be skipped
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "not-a-dir.txt"), []byte("x"), 0644))

	// Make it old
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(tmpDir, "not-a-dir.txt"), oldTime, oldTime))

	orphaned, err := DetectOrphanedDirectories(tmpDir, 1*time.Hour)
	require.NoError(t, err)
	assert.Empty(t, orphaned) // file should be skipped
}

func TestRemoveWithRetry_NonexistentPath(t *testing.T) {
	err := removeWithRetry("/nonexistent/path/that/does/not/exist", 3)
	assert.NoError(t, err) // should succeed because os.IsNotExist is OK
}

func TestCalculateDirSize_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	size := calculateDirSize(tmpDir)
	assert.Equal(t, int64(0), size)
}

func TestCalculateDirSize_NonexistentDir(t *testing.T) {
	size := calculateDirSize("/nonexistent/path")
	assert.Equal(t, int64(0), size)
}
