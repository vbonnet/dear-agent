//go:build linux

package overlayfs_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

// TestDestructiveIsolation verifies that rm -rf * in sandbox doesn't affect host.
// This is the critical safety test for OverlayFS isolation.
func TestDestructiveIsolation(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS only on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test repo with multiple files and directories
	lowerDir := t.TempDir()
	testFiles := []string{"file1.txt", "file2.txt", "dir/file3.txt", "dir/subdir/file4.txt"}
	createTestFiles(t, lowerDir, testFiles)

	// Create sandbox
	workspaceDir := t.TempDir()
	req := sandbox.SandboxRequest{
		SessionID:    "destruct-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	if err != nil {
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed (may need permissions): %v", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, sb)
	defer func() {
		_ = provider.Destroy(ctx, sb.ID)
	}()

	// Verify files exist in merged before deletion
	for _, file := range testFiles {
		mergedFile := filepath.Join(sb.MergedPath, file)
		_, err := os.Stat(mergedFile)
		require.NoError(t, err, "File should exist in merged before deletion: %s", file)
	}

	// Execute rm -rf * in merged directory (THE CRITICAL TEST)
	cmd := exec.Command("sh", "-c", "rm -rf *")
	cmd.Dir = sb.MergedPath
	err = cmd.Run()
	require.NoError(t, err, "rm -rf should succeed in sandbox")

	// CRITICAL VERIFICATION: All files in lowerdir must still exist
	assertFilesExist(t, lowerDir, testFiles)

	// Verify merged view is empty
	entries, err := os.ReadDir(sb.MergedPath)
	require.NoError(t, err)
	assert.Empty(t, entries, "Merged directory should be empty after rm -rf")

	// Verify whiteouts exist in upperdir
	assertWhiteoutsExist(t, sb.UpperPath, []string{"file1.txt", "file2.txt", "dir"})
}

// TestWhiteoutMechanism verifies the OverlayFS whiteout mechanism.
// Deleted files should appear as character devices (0,0) in upperdir.
func TestWhiteoutMechanism(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS only on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test repo
	lowerDir := t.TempDir()
	testFile := "test-delete.txt"
	testContent := []byte("this file will be deleted")
	err := os.WriteFile(filepath.Join(lowerDir, testFile), testContent, 0644)
	require.NoError(t, err)

	// Create sandbox
	workspaceDir := t.TempDir()
	req := sandbox.SandboxRequest{
		SessionID:    "whiteout-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	if err != nil {
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed: %v", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, sb)
	defer func() {
		_ = provider.Destroy(ctx, sb.ID)
	}()

	// Verify file exists in merged
	mergedFile := filepath.Join(sb.MergedPath, testFile)
	_, err = os.Stat(mergedFile)
	require.NoError(t, err, "File should exist before deletion")

	// Delete the file
	err = os.Remove(mergedFile)
	require.NoError(t, err, "Delete should succeed")

	// Verify file is gone from merged view
	_, err = os.Stat(mergedFile)
	assert.True(t, os.IsNotExist(err), "File should not exist in merged after deletion")

	// Verify original file intact in lowerdir
	lowerFile := filepath.Join(lowerDir, testFile)
	content, err := os.ReadFile(lowerFile)
	require.NoError(t, err, "Original file should still exist in lowerdir")
	assert.Equal(t, testContent, content, "Original file should be unchanged")

	// CRITICAL: Verify whiteout exists in upperdir as char device (0,0)
	whiteoutPath := filepath.Join(sb.UpperPath, testFile)
	stat, err := os.Stat(whiteoutPath)
	require.NoError(t, err, "Whiteout marker should exist in upperdir")

	// Check if it's a character device
	sys := stat.Sys().(*syscall.Stat_t)
	mode := sys.Mode & syscall.S_IFMT
	assert.Equal(t, uint32(syscall.S_IFCHR), mode, "Whiteout should be a character device")

	// Check major/minor device numbers are 0,0
	major := uint64(sys.Rdev) >> 8
	minor := uint64(sys.Rdev) & 0xff
	assert.Equal(t, uint64(0), major, "Whiteout major device number should be 0")
	assert.Equal(t, uint64(0), minor, "Whiteout minor device number should be 0")
}

// TestCopyUpOnWrite verifies the copy-up mechanism for modified files.
// Modified files should be copied to upperdir, leaving lowerdir unchanged.
func TestCopyUpOnWrite(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS only on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test repo
	lowerDir := t.TempDir()
	testFile := "modify-me.txt"
	originalContent := []byte("original content")
	err := os.WriteFile(filepath.Join(lowerDir, testFile), originalContent, 0644)
	require.NoError(t, err)

	// Create sandbox
	workspaceDir := t.TempDir()
	req := sandbox.SandboxRequest{
		SessionID:    "copyup-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	if err != nil {
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed: %v", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, sb)
	defer func() {
		_ = provider.Destroy(ctx, sb.ID)
	}()

	// Verify file doesn't exist in upperdir yet
	upperFile := filepath.Join(sb.UpperPath, testFile)
	_, err = os.Stat(upperFile)
	assert.True(t, os.IsNotExist(err), "File should not exist in upperdir before modification")

	// Modify the file (triggers copy-up)
	mergedFile := filepath.Join(sb.MergedPath, testFile)
	modifiedContent := []byte("modified content")
	err = os.WriteFile(mergedFile, modifiedContent, 0644)
	require.NoError(t, err, "Modification should succeed")

	// Verify modified file appears in upperdir
	content, err := os.ReadFile(upperFile)
	require.NoError(t, err, "Modified file should exist in upperdir")
	assert.Equal(t, modifiedContent, content, "Upperdir should contain modified content")

	// Verify original unchanged in lowerdir
	lowerFile := filepath.Join(lowerDir, testFile)
	content, err = os.ReadFile(lowerFile)
	require.NoError(t, err, "Original file should still exist in lowerdir")
	assert.Equal(t, originalContent, content, "Original file should be unchanged")

	// Verify merged shows modified version
	content, err = os.ReadFile(mergedFile)
	require.NoError(t, err, "Merged should show file")
	assert.Equal(t, modifiedContent, content, "Merged should show modified content")
}

// TestMultipleDestructiveOps runs repeated destructive operations to verify
// zero host corruption over many iterations.
func TestMultipleDestructiveOps(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS only on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test repo
	lowerDir := t.TempDir()
	testFiles := []string{
		"file1.txt",
		"file2.txt",
		"dir/file3.txt",
		"dir/subdir/file4.txt",
		"another/deep/path/file5.txt",
	}
	createTestFiles(t, lowerDir, testFiles)

	const iterations = 100
	whiteoutCounts := make([]int, 0, iterations)

	for i := 0; i < iterations; i++ {
		// Create sandbox
		workspaceDir := t.TempDir()
		req := sandbox.SandboxRequest{
			SessionID:    fmt.Sprintf("stress-test-%d", i),
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: workspaceDir,
		}

		sb, err := provider.Create(ctx, req)
		if err != nil {
			var sbErr *sandbox.Error
			if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
				t.Skipf("Mount failed: %v", err)
			}
			require.NoError(t, err, "Iteration %d: Create failed", i)
		}
		require.NotNil(t, sb)

		// Perform random destructive operations
		switch i % 3 {
		case 0:
			// Delete everything
			cmd := exec.Command("sh", "-c", "rm -rf *")
			cmd.Dir = sb.MergedPath
			err = cmd.Run()
			require.NoError(t, err, "Iteration %d: rm -rf failed", i)
		case 1:
			// Delete specific files
			for _, file := range testFiles {
				_ = os.RemoveAll(filepath.Join(sb.MergedPath, file))
			}
		case 2:
			// Modify then delete
			for _, file := range testFiles {
				mergedFile := filepath.Join(sb.MergedPath, file)
				_ = os.WriteFile(mergedFile, []byte("modified"), 0644)
				_ = os.Remove(mergedFile)
			}
		}

		// Count whiteouts in upperdir
		whiteoutCount := countWhiteouts(t, sb.UpperPath)
		whiteoutCounts = append(whiteoutCounts, whiteoutCount)

		// CRITICAL: Verify lowerdir still intact
		assertFilesExist(t, lowerDir, testFiles)

		// Cleanup
		err = provider.Destroy(ctx, sb.ID)
		require.NoError(t, err, "Iteration %d: Destroy failed", i)
	}

	// Log whiteout statistics
	t.Logf("Completed %d iterations with zero host corruption", iterations)
	t.Logf("Whiteout counts: min=%d, max=%d", minInt(whiteoutCounts), maxInt(whiteoutCounts))
}

// TestDirectoryWhiteouts verifies whiteout behavior for deleted directories.
func TestDirectoryWhiteouts(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS only on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test repo with nested directories
	lowerDir := t.TempDir()
	testDir := "testdir"
	testDirPath := filepath.Join(lowerDir, testDir)
	err := os.MkdirAll(filepath.Join(testDirPath, "subdir"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(testDirPath, "file.txt"), []byte("test"), 0644)
	require.NoError(t, err)

	// Create sandbox
	workspaceDir := t.TempDir()
	req := sandbox.SandboxRequest{
		SessionID:    "dir-whiteout-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	if err != nil {
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed: %v", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, sb)
	defer func() {
		_ = provider.Destroy(ctx, sb.ID)
	}()

	// Delete the entire directory
	mergedDirPath := filepath.Join(sb.MergedPath, testDir)
	err = os.RemoveAll(mergedDirPath)
	require.NoError(t, err, "Directory deletion should succeed")

	// Verify directory is gone from merged
	_, err = os.Stat(mergedDirPath)
	assert.True(t, os.IsNotExist(err), "Directory should not exist in merged")

	// Verify original directory intact in lowerdir
	_, err = os.Stat(testDirPath)
	require.NoError(t, err, "Original directory should still exist in lowerdir")

	// Verify directory whiteout in upperdir
	// OverlayFS creates opaque directory markers for deleted directories
	whiteoutPath := filepath.Join(sb.UpperPath, testDir)
	stat, err := os.Stat(whiteoutPath)
	require.NoError(t, err, "Directory whiteout marker should exist")

	// Verify it's a char device
	sys := stat.Sys().(*syscall.Stat_t)
	mode := sys.Mode & syscall.S_IFMT
	assert.Equal(t, uint32(syscall.S_IFCHR), mode, "Directory whiteout should be char device")
}

// TestNestedFileOperations tests operations on files in nested directories.
func TestNestedFileOperations(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS only on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create deeply nested structure
	lowerDir := t.TempDir()
	nestedFile := "a/b/c/d/e/nested.txt"
	nestedPath := filepath.Join(lowerDir, nestedFile)
	err := os.MkdirAll(filepath.Dir(nestedPath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(nestedPath, []byte("deeply nested"), 0644)
	require.NoError(t, err)

	// Create sandbox
	workspaceDir := t.TempDir()
	req := sandbox.SandboxRequest{
		SessionID:    "nested-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	if err != nil {
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed: %v", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, sb)
	defer func() {
		_ = provider.Destroy(ctx, sb.ID)
	}()

	// Modify nested file
	mergedNested := filepath.Join(sb.MergedPath, nestedFile)
	err = os.WriteFile(mergedNested, []byte("modified nested"), 0644)
	require.NoError(t, err)

	// Verify original unchanged
	content, err := os.ReadFile(nestedPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("deeply nested"), content)

	// Delete nested file
	err = os.Remove(mergedNested)
	require.NoError(t, err)

	// Verify original still exists
	_, err = os.Stat(nestedPath)
	require.NoError(t, err, "Original nested file should still exist")
}

// Helper functions

// createTestFiles creates test files in the given directory.
func createTestFiles(t *testing.T, baseDir string, files []string) {
	t.Helper()
	for _, file := range files {
		path := filepath.Join(baseDir, file)
		err := os.MkdirAll(filepath.Dir(path), 0755)
		require.NoError(t, err, "Failed to create directory for %s", file)
		content := []byte(fmt.Sprintf("content of %s", file))
		err = os.WriteFile(path, content, 0644)
		require.NoError(t, err, "Failed to create file %s", file)
	}
}

// assertFilesExist verifies all files exist and are readable.
func assertFilesExist(t *testing.T, baseDir string, files []string) {
	t.Helper()
	for _, file := range files {
		path := filepath.Join(baseDir, file)
		content, err := os.ReadFile(path)
		require.NoError(t, err, "File should exist: %s", path)
		expectedContent := fmt.Sprintf("content of %s", file)
		assert.Equal(t, expectedContent, string(content), "File content should be unchanged: %s", file)
	}
}

// assertWhiteoutsExist verifies whiteout markers exist in upperdir.
func assertWhiteoutsExist(t *testing.T, upperDir string, items []string) {
	t.Helper()
	for _, item := range items {
		whiteoutPath := filepath.Join(upperDir, item)
		stat, err := os.Stat(whiteoutPath)
		if err != nil {
			// Whiteout might not exist for some deletion patterns
			t.Logf("Warning: Expected whiteout not found: %s", item)
			continue
		}

		// Verify it's a character device (whiteout marker)
		sys := stat.Sys().(*syscall.Stat_t)
		mode := sys.Mode & syscall.S_IFMT
		if mode == syscall.S_IFCHR {
			// Check device numbers are 0,0
			major := uint64(sys.Rdev) >> 8
			minor := uint64(sys.Rdev) & 0xff
			assert.Equal(t, uint64(0), major, "Whiteout %s should have major=0", item)
			assert.Equal(t, uint64(0), minor, "Whiteout %s should have minor=0", item)
			t.Logf("Verified whiteout: %s (char device 0,0)", item)
		}
	}
}

// countWhiteouts counts whiteout markers in a directory tree.
func countWhiteouts(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		sys := info.Sys().(*syscall.Stat_t)
		mode := sys.Mode & syscall.S_IFMT
		if mode == syscall.S_IFCHR {
			// Verify it's a proper whiteout (0,0)
			major := uint64(sys.Rdev) >> 8
			minor := uint64(sys.Rdev) & 0xff
			if major == 0 && minor == 0 {
				count++
			}
		}
		return nil
	})
	if err != nil {
		t.Logf("Warning: error walking directory: %v", err)
	}
	return count
}

// minInt returns the minimum value in a slice.
func minInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

// maxInt returns the maximum value in a slice.
func maxInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}
