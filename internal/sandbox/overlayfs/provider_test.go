package overlayfs_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

// TestOverlayFSProvider runs standard contract tests.
func TestOverlayFSProvider(t *testing.T) {
	// Skip if not on Linux
	if !isLinux() {
		t.Skip("OverlayFS tests only run on Linux")
	}

	provider := overlayfs.NewProvider()
	require.NotNil(t, provider, "NewProvider should return non-nil")

	// Run basic tests that don't require root
	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "overlayfs-native", provider.Name())
	})

	t.Run("Validate_nonexistent", func(t *testing.T) {
		ctx := context.Background()
		err := provider.Validate(ctx, "nonexistent-sandbox")
		require.Error(t, err)

		var sbErr *sandbox.Error
		if assert.True(t, errors.As(err, &sbErr)) {
			assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code)
		}
	})

	t.Run("Destroy_idempotent", func(t *testing.T) {
		ctx := context.Background()
		err := provider.Destroy(ctx, "nonexistent-sandbox")
		assert.NoError(t, err, "Destroy should be idempotent")
	})
}

// TestOverlayFSCreate tests full create/destroy lifecycle.
// Requires kernel 5.11+ and may require permissions.
func TestOverlayFSCreate(t *testing.T) {
	// Skip if not on Linux
	if !isLinux() {
		t.Skip("OverlayFS tests only run on Linux")
	}

	// Skip if kernel too old
	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create test directories
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Create test file in lower directory
	testFile := filepath.Join(lowerDir, "test.txt")
	err := os.WriteFile(testFile, []byte("hello world"), 0644)
	require.NoError(t, err, "Failed to create test file")

	req := sandbox.SandboxRequest{
		SessionID:    "test-overlayfs-session",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"TEST_KEY": "test_value",
		},
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	if err != nil {
		// If mount fails due to permissions, skip test
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed (may need permissions): %v", err)
		}
		require.NoError(t, err, "Create should succeed")
	}
	require.NotNil(t, sb, "Sandbox should not be nil")

	// Verify sandbox structure
	assert.Equal(t, req.SessionID, sb.ID)
	assert.NotEmpty(t, sb.MergedPath)
	assert.NotEmpty(t, sb.UpperPath)
	assert.NotEmpty(t, sb.WorkPath)
	assert.Equal(t, "overlayfs-native", sb.Type)

	// Verify merged directory contains lower file
	mergedFile := filepath.Join(sb.MergedPath, "test.txt")
	content, err := os.ReadFile(mergedFile)
	assert.NoError(t, err, "Should be able to read file from merged")
	assert.Equal(t, "hello world", string(content))

	// Verify secrets file exists
	envFile := filepath.Join(sb.UpperPath, ".env")
	_, err = os.Stat(envFile)
	assert.NoError(t, err, "Secrets file should exist")

	// Validate sandbox
	err = provider.Validate(ctx, sb.ID)
	assert.NoError(t, err, "Validate should succeed")

	// Modify file (triggers copy-up)
	modifiedContent := []byte("modified")
	err = os.WriteFile(mergedFile, modifiedContent, 0644)
	require.NoError(t, err, "Should be able to modify file")

	// Verify modification is in merged view
	content, err = os.ReadFile(mergedFile)
	assert.NoError(t, err)
	assert.Equal(t, "modified", string(content))

	// Verify original lower file is unchanged
	content, err = os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", string(content), "Lower file should be unchanged")

	// Destroy sandbox
	err = provider.Destroy(ctx, sb.ID)
	require.NoError(t, err, "Destroy should succeed")

	// Verify merged directory is gone or empty
	_, err = os.Stat(sb.MergedPath)
	if err == nil {
		// Directory might still exist but should be empty
		entries, _ := os.ReadDir(sb.MergedPath)
		assert.Empty(t, entries, "Merged directory should be empty after destroy")
	}

	// Validate should fail after destroy
	err = provider.Validate(ctx, sb.ID)
	assert.Error(t, err, "Validate should fail after Destroy")
}

// TestOverlayFSMultipleRepos tests mounting multiple lower directories.
func TestOverlayFSMultipleRepos(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS tests only run on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	// Create multiple lower directories
	lowerDir1 := t.TempDir()
	lowerDir2 := t.TempDir()
	workspaceDir := t.TempDir()

	// Create test files in each lower directory
	err := os.WriteFile(filepath.Join(lowerDir1, "file1.txt"), []byte("repo1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(lowerDir2, "file2.txt"), []byte("repo2"), 0644)
	require.NoError(t, err)

	req := sandbox.SandboxRequest{
		SessionID:    "test-multi-repo",
		LowerDirs:    []string{lowerDir1, lowerDir2},
		WorkspaceDir: workspaceDir,
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	if err != nil {
		var sbErr *sandbox.Error
		if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
			t.Skipf("Mount failed: %v", err)
		}
		require.NoError(t, err)
	}
	require.NotNil(t, sb)

	// Verify both files are visible in merged view
	file1 := filepath.Join(sb.MergedPath, "file1.txt")
	file2 := filepath.Join(sb.MergedPath, "file2.txt")

	content1, err := os.ReadFile(file1)
	assert.NoError(t, err)
	assert.Equal(t, "repo1", string(content1))

	content2, err := os.ReadFile(file2)
	assert.NoError(t, err)
	assert.Equal(t, "repo2", string(content2))

	// Cleanup
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestOverlayFSConcurrency tests creating multiple sandboxes concurrently.
func TestOverlayFSConcurrency(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS tests only run on Linux")
	}

	if !hasKernel511() {
		t.Skip("OverlayFS rootless requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	const numSandboxes = 5
	results := make(chan error, numSandboxes)
	sandboxes := make(chan *sandbox.Sandbox, numSandboxes)

	// Create multiple sandboxes concurrently
	for i := 0; i < numSandboxes; i++ {
		go func(id int) {
			lowerDir := t.TempDir()
			workspaceDir := t.TempDir()

			req := sandbox.SandboxRequest{
				SessionID:    fmt.Sprintf("concurrent-test-%d", id),
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			}

			sb, err := provider.Create(ctx, req)
			if err != nil {
				var sbErr *sandbox.Error
				if errors.As(err, &sbErr) && sbErr.Code == sandbox.ErrCodeMountFailed {
					// Skip if mount fails
					results <- nil
					return
				}
			}
			results <- err
			sandboxes <- sb
		}(i)
	}

	// Collect results
	var created []*sandbox.Sandbox
	for i := 0; i < numSandboxes; i++ {
		err := <-results
		if err == nil {
			select {
			case sb := <-sandboxes:
				if sb != nil {
					created = append(created, sb)
				}
			default:
			}
		}
	}

	// Cleanup all created sandboxes
	for _, sb := range created {
		err := provider.Destroy(ctx, sb.ID)
		assert.NoError(t, err)
	}

	if len(created) > 0 {
		t.Logf("Successfully created and destroyed %d concurrent sandboxes", len(created))
	}
}

// TestOverlayFSContextCancellation tests context cancellation during create.
func TestOverlayFSContextCancellation(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS tests only run on Linux")
	}

	provider := overlayfs.NewProvider()

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := sandbox.SandboxRequest{
		SessionID:    "test-cancelled",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	// Should fail due to cancelled context
	_, err := provider.Create(ctx, req)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

// TestOverlayFSInvalidRequest tests error handling for invalid requests.
func TestOverlayFSInvalidRequest(t *testing.T) {
	if !isLinux() {
		t.Skip("OverlayFS tests only run on Linux")
	}

	provider := overlayfs.NewProvider()
	ctx := context.Background()

	tests := []struct {
		name    string
		req     sandbox.SandboxRequest
		wantErr sandbox.ErrorCode
	}{
		{
			name: "empty_session_id",
			req: sandbox.SandboxRequest{
				SessionID:    "",
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: t.TempDir(),
			},
			wantErr: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "no_lower_dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{},
				WorkspaceDir: t.TempDir(),
			},
			wantErr: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "nonexistent_lower_dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/nonexistent/path"},
				WorkspaceDir: t.TempDir(),
			},
			wantErr: sandbox.ErrCodeRepoNotFound,
		},
		{
			name: "empty_workspace_dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: "",
			},
			wantErr: sandbox.ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.Create(ctx, tt.req)
			require.Error(t, err)

			var sbErr *sandbox.Error
			if assert.True(t, errors.As(err, &sbErr)) {
				assert.Equal(t, tt.wantErr, sbErr.Code)
			}
		})
	}
}

// Helper functions (test-only utilities for overlayfs_test package)

func isLinux() bool {
	return runtime.GOOS == "linux"
}

func hasKernel511() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	version := parseKernelVersion(string(data))
	return isKernelVersionAtLeast(version, 5, 11)
}

func parseKernelVersion(versionStr string) string {
	parts := strings.Fields(versionStr)
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			version := parts[i+1]
			version = strings.TrimRight(version, "+-")
			return version
		}
	}
	return "unknown"
}

func isKernelVersionAtLeast(version string, major, minor int) bool {
	var vMajor, vMinor int
	_, err := fmt.Sscanf(version, "%d.%d", &vMajor, &vMinor)
	if err != nil {
		return false
	}

	if vMajor > major {
		return true
	}
	if vMajor == major && vMinor >= minor {
		return true
	}
	return false
}
