//go:build linux

package overlayfs_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

// TestOverlayFS_E2E tests end-to-end OverlayFS lifecycle
func TestOverlayFS_E2E(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OverlayFS only available on Linux")
	}
	if os.Getuid() != 0 {
		t.Skip("overlayfs requires root")
	}

	provider := overlayfs.NewProvider()
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
	// CRITICAL: Do NOT skip on mount failures - tests must FAIL if mounts don't work
	// This prevents false confidence from passing tests that actually skipped
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Sandbox creation must succeed - if mount fails, test should FAIL not skip")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify sandbox structure
	t.Run("sandbox_structure", func(t *testing.T) {
		assert.DirExists(t, sb.MergedPath)
		assert.DirExists(t, sb.UpperPath)
		assert.DirExists(t, sb.WorkPath)
		assert.Equal(t, "overlayfs-native", sb.Type)
	})

	// Verify files visible in merged
	t.Run("files_visible_in_merged", func(t *testing.T) {
		for path := range testFiles {
			mergedFile := filepath.Join(sb.MergedPath, path)
			_, err := os.Stat(mergedFile)
			assert.NoError(t, err, "File should exist in merged: %s", path)
		}
	})

	// Verify secrets written to upperdir/.env
	t.Run("secrets_injection", func(t *testing.T) {
		envFile := filepath.Join(sb.UpperPath, ".env")
		content, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "API_KEY=test-key-123")
	})

	// Test modification creates copy in upperdir
	t.Run("copy_on_write", func(t *testing.T) {
		mergedFile := filepath.Join(sb.MergedPath, "README.md")
		err := os.WriteFile(mergedFile, []byte("# Modified"), 0644)
		require.NoError(t, err)

		// File should exist in upperdir
		upperFile := filepath.Join(sb.UpperPath, "README.md")
		_, err = os.Stat(upperFile)
		assert.NoError(t, err, "Modified file should be copied to upperdir")

		// Original should be unchanged
		lowerFile := filepath.Join(lowerDir, "README.md")
		original, err := os.ReadFile(lowerFile)
		require.NoError(t, err)
		assert.Equal(t, "# Test Repository", string(original))
	})

	// Test deletion creates whiteout
	t.Run("deletion_whiteout", func(t *testing.T) {
		mergedFile := filepath.Join(sb.MergedPath, "config/settings.yml")
		err := os.Remove(mergedFile)
		require.NoError(t, err)

		// File should not be visible in merged
		_, err = os.Stat(mergedFile)
		assert.True(t, os.IsNotExist(err))

		// Original should still exist in lowerdir
		lowerFile := filepath.Join(lowerDir, "config/settings.yml")
		_, err = os.Stat(lowerFile)
		assert.NoError(t, err, "Original file should still exist in lowerdir")

		// Whiteout should exist in upperdir
		whiteoutPath := filepath.Join(sb.UpperPath, "config/settings.yml")
		info, err := os.Lstat(whiteoutPath)
		if err == nil {
			// Whiteout is a character device (0,0)
			stat, ok := info.Sys().(*syscall.Stat_t)
			if ok && stat.Rdev == 0 && (info.Mode()&os.ModeCharDevice) != 0 {
				t.Logf("Whiteout detected for deleted file")
			}
		}
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

// TestOverlayFS_MultiRepo tests with multiple lower directories
func TestOverlayFS_MultiRepo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OverlayFS only available on Linux")
	}
	if os.Getuid() != 0 {
		t.Skip("overlayfs requires root")
	}

	provider := overlayfs.NewProvider()
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

	// Create overlapping file (should prioritize first lower dir)
	err = os.WriteFile(filepath.Join(repo1, "shared.txt"), []byte("from-repo1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(repo2, "shared.txt"), []byte("from-repo2"), 0644)
	require.NoError(t, err)

	req := sandbox.SandboxRequest{
		SessionID:    "multi-repo-test",
		LowerDirs:    []string{repo1, repo2, repo3},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Sandbox creation must succeed - if mount fails, test should FAIL not skip")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify all files visible in merged
	t.Run("all_repos_visible", func(t *testing.T) {
		files := []string{"file1.txt", "file2.txt", "file3.txt"}
		for _, file := range files {
			mergedFile := filepath.Join(sb.MergedPath, file)
			_, err := os.Stat(mergedFile)
			assert.NoError(t, err, "File from repo should be visible: %s", file)
		}
	})

	// Verify priority order (first lowerdir wins)
	t.Run("priority_order", func(t *testing.T) {
		sharedFile := filepath.Join(sb.MergedPath, "shared.txt")
		content, err := os.ReadFile(sharedFile)
		require.NoError(t, err)
		assert.Equal(t, "from-repo1", string(content), "First lowerdir should have priority")
	})

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestOverlayFS_Isolation verifies destructive operations don't affect host
func TestOverlayFS_Isolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OverlayFS only available on Linux")
	}
	if os.Getuid() != 0 {
		t.Skip("overlayfs requires root")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test repository
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	testFiles := []string{"file1.txt", "file2.txt", "dir/file3.txt"}
	for _, file := range testFiles {
		fullPath := filepath.Join(lowerDir, file)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte("content"), 0644)
		require.NoError(t, err)
	}

	req := sandbox.SandboxRequest{
		SessionID:    "isolation-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Sandbox creation must succeed - if mount fails, test should FAIL not skip")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Execute rm -rf * in sandbox (THE CRITICAL TEST)
	t.Run("destructive_operation_isolation", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "rm -rf *")
		cmd.Dir = sb.MergedPath
		err := cmd.Run()
		require.NoError(t, err, "rm -rf should succeed in sandbox")

		// CRITICAL: Verify all files still exist in lowerdir
		for _, file := range testFiles {
			lowerFile := filepath.Join(lowerDir, file)
			_, err := os.Stat(lowerFile)
			assert.NoError(t, err, "Original file should still exist after rm -rf: %s", file)
		}

		// Verify merged is empty
		entries, err := os.ReadDir(sb.MergedPath)
		require.NoError(t, err)
		assert.Empty(t, entries, "Merged should be empty after rm -rf")
	})

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestOverlayFS_ContextCancellation tests context handling
func TestOverlayFS_ContextCancellation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OverlayFS only available on Linux")
	}

	provider := overlayfs.NewProvider()

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

// TestOverlayFS_ValidationErrors tests validation error handling
func TestOverlayFS_ValidationErrors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OverlayFS only available on Linux")
	}

	provider := overlayfs.NewProvider()
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

// TestOverlayFS_IdempotentDestroy tests destroy idempotency
func TestOverlayFS_IdempotentDestroy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OverlayFS only available on Linux")
	}
	if os.Getuid() != 0 {
		t.Skip("overlayfs requires root")
	}

	provider := overlayfs.NewProvider()
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
	require.NoError(t, err, "Sandbox creation must succeed - if mount fails, test should FAIL not skip")
	require.NotNil(t, sb)

	// First destroy
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)

	// Second destroy
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err, "Second destroy should also succeed")
}
