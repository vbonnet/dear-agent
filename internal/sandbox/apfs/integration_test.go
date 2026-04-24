//go:build darwin

package apfs_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/apfs"
)

// TestAPFS_E2E tests end-to-end APFS lifecycle
func TestAPFS_E2E(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()
	ctx := context.Background()

	// Create test repository with sample files
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Create test files in lowerdir
	testFiles := map[string]string{
		"README.md":           "# Test Repository",
		"src/main.go":         "package main\n\nfunc main() {}",
		"src/lib/helper.go":   "package lib\n\nfunc Helper() {}",
		"config/settings.yml": "debug: true",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(lowerDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	req := sandbox.SandboxRequest{
		SessionID:    "e2e-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"API_KEY": "test-key-123",
		},
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify sandbox structure
	t.Run("sandbox_structure", func(t *testing.T) {
		assert.DirExists(t, sb.MergedPath)
		assert.DirExists(t, sb.UpperPath)
		assert.Equal(t, "apfs-reflink", sb.Type)
		assert.Empty(t, sb.WorkPath, "APFS doesn't use work directory")
	})

	// Verify merged is symlink to upperdir
	t.Run("merged_is_symlink", func(t *testing.T) {
		target, err := os.Readlink(sb.MergedPath)
		require.NoError(t, err)
		assert.Equal(t, sb.UpperPath, target)
	})

	// Verify files visible in merged
	t.Run("files_visible_in_merged", func(t *testing.T) {
		for path := range testFiles {
			mergedFile := filepath.Join(sb.MergedPath, "repo0", path)
			_, err := os.Stat(mergedFile)
			assert.NoError(t, err, "File should exist in merged: %s", path)
		}
	})

	// Verify secrets written to upperdir/.env
	t.Run("secrets_injection", func(t *testing.T) {
		envFile := filepath.Join(sb.UpperPath, ".env")
		content, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "API_KEY=test_key-123")
	})

	// Test modification in cloned directory
	t.Run("modification_in_clone", func(t *testing.T) {
		clonedFile := filepath.Join(sb.UpperPath, "repo0", "README.md")
		err := os.WriteFile(clonedFile, []byte("# Modified"), 0644)
		require.NoError(t, err)

		// Original should be unchanged (CoW semantics)
		lowerFile := filepath.Join(lowerDir, "README.md")
		original, err := os.ReadFile(lowerFile)
		require.NoError(t, err)
		assert.Equal(t, "# Test Repository", string(original))
	})

	// Validate sandbox
	err = provider.Validate(ctx, sb.ID)
	assert.NoError(t, err)

	// Destroy sandbox
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)

	// Verify cleanup
	_, err = os.Stat(sb.MergedPath)
	assert.True(t, os.IsNotExist(err), "Merged path should be removed after destroy")
}

// TestAPFS_Reflink tests APFS reflink cloning works correctly
func TestAPFS_Reflink(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()
	ctx := context.Background()

	// Create test repository with large file
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Create a 1MB test file
	largeFile := filepath.Join(lowerDir, "large.bin")
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	err := os.WriteFile(largeFile, data, 0644)
	require.NoError(t, err)

	req := sandbox.SandboxRequest{
		SessionID:    "reflink-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify file was cloned
	t.Run("file_cloned", func(t *testing.T) {
		clonedFile := filepath.Join(sb.UpperPath, "repo0", "large.bin")
		clonedData, err := os.ReadFile(clonedFile)
		require.NoError(t, err)
		assert.Equal(t, data, clonedData, "Cloned file should have same content")
	})

	// Verify CoW: modify clone doesn't affect original
	t.Run("copy_on_write", func(t *testing.T) {
		clonedFile := filepath.Join(sb.UpperPath, "repo0", "large.bin")

		// Modify first byte
		f, err := os.OpenFile(clonedFile, os.O_RDWR, 0644)
		require.NoError(t, err)
		_, err = f.WriteAt([]byte{0xFF}, 0)
		require.NoError(t, err)
		f.Close()

		// Original should be unchanged
		originalData, err := os.ReadFile(largeFile)
		require.NoError(t, err)
		assert.Equal(t, byte(0), originalData[0], "Original file should be unchanged")

		// Clone should be modified
		modifiedData, err := os.ReadFile(clonedFile)
		require.NoError(t, err)
		assert.Equal(t, byte(0xFF), modifiedData[0], "Cloned file should be modified")
	})

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestAPFS_Fallback tests fallback to recursive copy on non-APFS
func TestAPFS_Fallback(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	// This test documents fallback behavior
	// On APFS volumes, cp -c should work
	// On non-APFS volumes (HFS+, NFS, SMB), should fall back to recursive copy

	provider := apfs.NewProvider()
	ctx := context.Background()

	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(lowerDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	req := sandbox.SandboxRequest{
		SessionID:    "fallback-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	// Create sandbox - should work on both APFS and non-APFS
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Should work on both APFS and non-APFS")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify file was copied/cloned
	clonedFile := filepath.Join(sb.UpperPath, "repo0", "test.txt")
	content, err := os.ReadFile(clonedFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestAPFS_MultiRepo tests with multiple lower directories
func TestAPFS_MultiRepo(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()
	ctx := context.Background()

	// Create multiple repositories
	repo1 := t.TempDir()
	repo2 := t.TempDir()
	repo3 := t.TempDir()
	workspaceDir := t.TempDir()

	// Create files in each repo
	err := os.WriteFile(filepath.Join(repo1, "file1.txt"), []byte("repo1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(repo2, "file2.txt"), []byte("repo2"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(repo3, "file3.txt"), []byte("repo3"), 0644)
	require.NoError(t, err)

	req := sandbox.SandboxRequest{
		SessionID:    "multi-repo-test",
		LowerDirs:    []string{repo1, repo2, repo3},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify all repos cloned
	t.Run("all_repos_cloned", func(t *testing.T) {
		files := map[string]string{
			"repo0/file1.txt": "repo1",
			"repo1/file2.txt": "repo2",
			"repo2/file3.txt": "repo3",
		}
		for path, expected := range files {
			clonedFile := filepath.Join(sb.UpperPath, path)
			content, err := os.ReadFile(clonedFile)
			require.NoError(t, err)
			assert.Equal(t, expected, string(content))
		}
	})

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestAPFS_ContextCancellation tests context handling
func TestAPFS_ContextCancellation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()

	t.Run("cancelled_before_create", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := sandbox.SandboxRequest{
			SessionID:    "cancel-test",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}

		_, err := provider.Create(ctx, req)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("timeout_during_create", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond) // Ensure timeout

		req := sandbox.SandboxRequest{
			SessionID:    "timeout-test",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}

		_, err := provider.Create(ctx, req)
		assert.Error(t, err)
	})
}

// TestAPFS_ValidationErrors tests validation error handling
func TestAPFS_ValidationErrors(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()
	ctx := context.Background()

	tests := []struct {
		name      string
		req       sandbox.SandboxRequest
		wantError bool
		errorCode sandbox.ErrorCode
	}{
		{
			name: "empty_session_id",
			req: sandbox.SandboxRequest{
				SessionID:    "",
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: t.TempDir(),
			},
			wantError: true,
			errorCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "empty_lower_dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{},
				WorkspaceDir: t.TempDir(),
			},
			wantError: true,
			errorCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "nonexistent_lower_dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/nonexistent/path"},
				WorkspaceDir: t.TempDir(),
			},
			wantError: true,
			errorCode: sandbox.ErrCodeRepoNotFound,
		},
		{
			name: "empty_workspace_dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: "",
			},
			wantError: true,
			errorCode: sandbox.ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.Create(ctx, tt.req)
			if tt.wantError {
				require.Error(t, err)
				var sbErr *sandbox.Error
				if assert.ErrorAs(t, err, &sbErr) {
					assert.Equal(t, tt.errorCode, sbErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestAPFS_IdempotentDestroy tests destroy idempotency
func TestAPFS_IdempotentDestroy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()
	ctx := context.Background()

	// Destroy non-existent sandbox should succeed
	err := provider.Destroy(ctx, "nonexistent-sandbox")
	assert.NoError(t, err, "Destroy should be idempotent")

	// Destroy same sandbox multiple times should succeed
	req := sandbox.SandboxRequest{
		SessionID:    "idempotent-test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, sb)

	// First destroy
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)

	// Second destroy
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err, "Second destroy should also succeed")
}

// TestAPFS_SymlinkValidation tests symlink validation
func TestAPFS_SymlinkValidation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS only available on macOS")
	}

	provider := apfs.NewProvider()
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "symlink-test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Validate should check symlink integrity
	err = provider.Validate(ctx, sb.ID)
	assert.NoError(t, err)

	// Remove symlink target
	err = os.RemoveAll(sb.UpperPath)
	require.NoError(t, err)

	// Validate should fail
	err = provider.Validate(ctx, sb.ID)
	assert.Error(t, err, "Validate should fail when symlink target is missing")

	var sbErr *sandbox.Error
	if assert.ErrorAs(t, err, &sbErr) {
		assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code)
	}
}
