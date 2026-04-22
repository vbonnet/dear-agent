package bubblewrap_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/bubblewrap"
)

// TestBubblewrap_E2E tests end-to-end Bubblewrap lifecycle
func TestBubblewrap_E2E(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Bubblewrap only available on Linux")
	}

	// Check if bwrap is installed
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("Bubblewrap (bwrap) not installed")
	}

	provider := bubblewrap.NewProvider()
	ctx := context.Background()

	// Create test repository
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

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
		SessionID:    "bwrap-e2e-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"API_KEY": "test-key-123",
		},
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Bubblewrap sandbox creation must succeed")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	// Verify sandbox structure
	t.Run("sandbox_structure", func(t *testing.T) {
		assert.DirExists(t, sb.MergedPath)
		assert.DirExists(t, sb.UpperPath)
		assert.DirExists(t, sb.WorkPath)
		assert.Equal(t, "bubblewrap", sb.Type)
	})

	// Verify secrets written to upperdir/.env
	t.Run("secrets_injection", func(t *testing.T) {
		envFile := filepath.Join(sb.UpperPath, ".env")
		content, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "API_KEY=test-key-123")
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

// TestBubblewrap_MultiRepo tests with multiple lower directories
func TestBubblewrap_MultiRepo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Bubblewrap only available on Linux")
	}

	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("Bubblewrap (bwrap) not installed")
	}

	provider := bubblewrap.NewProvider()
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
		SessionID:    "bwrap-multi-repo-test",
		LowerDirs:    []string{repo1, repo2, repo3},
		WorkspaceDir: workspaceDir,
	}

	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Multi-repo sandbox creation must succeed")
	require.NotNil(t, sb)
	defer provider.Destroy(ctx, sb.ID)

	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err)
}

// TestBubblewrap_ValidationErrors tests validation error handling
func TestBubblewrap_ValidationErrors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Bubblewrap only available on Linux")
	}

	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("Bubblewrap (bwrap) not installed")
	}

	provider := bubblewrap.NewProvider()
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

// TestBubblewrap_IdempotentDestroy tests destroy idempotency
func TestBubblewrap_IdempotentDestroy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Bubblewrap only available on Linux")
	}

	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("Bubblewrap (bwrap) not installed")
	}

	provider := bubblewrap.NewProvider()
	ctx := context.Background()

	// Destroy non-existent sandbox should succeed
	err := provider.Destroy(ctx, "nonexistent-sandbox")
	assert.NoError(t, err, "Destroy should be idempotent")

	// Destroy same sandbox multiple times should succeed
	req := sandbox.SandboxRequest{
		SessionID:    "bwrap-idempotent-test",
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
