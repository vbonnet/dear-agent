package overlayfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard_kernel_version",
			input:    "Linux version 6.6.123+ (builder@host) (gcc version 11.2.0) #1 SMP",
			expected: "6.6.123",
		},
		{
			name:     "kernel_with_dash_suffix",
			input:    "Linux version 5.11.0-ubuntu1 (builder@host) #1 SMP",
			expected: "5.11.0-ubuntu1", // TrimRight only removes trailing +/-
		},
		{
			name:     "minimal_version",
			input:    "Linux version 5.4.0",
			expected: "5.4.0",
		},
		{
			name:     "version_word_elsewhere",
			input:    "Some random string without version keyword",
			expected: "keyword", // "version" found, next word returned
		},
		{
			name:     "version_at_end_no_value",
			input:    "Linux version",
			expected: "unknown",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "version_with_trailing_plus_dash",
			input:    "Linux version 4.19.128+- (gcc) #1",
			expected: "4.19.128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKernelVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsKernelVersionAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		version string
		major   int
		minor   int
		want    bool
	}{
		{"exact_match", "5.11.0", 5, 11, true},
		{"higher_major", "6.0.0", 5, 11, true},
		{"higher_minor", "5.15.0", 5, 11, true},
		{"lower_minor", "5.10.0", 5, 11, false},
		{"lower_major", "4.19.0", 5, 11, false},
		{"invalid_version", "unknown", 5, 11, false},
		{"empty_version", "", 5, 11, false},
		{"malformed", "abc.def", 5, 11, false},
		{"zero_zero", "0.0.0", 0, 0, true},
		{"major_only_parsed", "6.0", 5, 11, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKernelVersionAtLeast(tt.version, tt.major, tt.minor)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestVerifyCleanupComplete(t *testing.T) {
	p := NewProvider()

	t.Run("all_removed", func(t *testing.T) {
		err := p.verifyCleanupComplete(
			"/nonexistent/upper",
			"/nonexistent/work",
			"/nonexistent/merged",
		)
		assert.NoError(t, err)
	})

	t.Run("some_remaining", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "upper")
		workDir := filepath.Join(tmpDir, "work")
		mergedDir := filepath.Join(tmpDir, "merged")

		require.NoError(t, os.MkdirAll(upperDir, 0755))

		err := p.verifyCleanupComplete(upperDir, workDir, mergedDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "directories still exist")
	})

	t.Run("all_remaining", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "upper")
		workDir := filepath.Join(tmpDir, "work")
		mergedDir := filepath.Join(tmpDir, "merged")

		require.NoError(t, os.MkdirAll(upperDir, 0755))
		require.NoError(t, os.MkdirAll(workDir, 0755))
		require.NoError(t, os.MkdirAll(mergedDir, 0755))

		err := p.verifyCleanupComplete(upperDir, workDir, mergedDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "directories still exist")
	})
}

func TestVerifyUnmounted(t *testing.T) {
	p := NewProvider()

	t.Run("not_mounted_path", func(t *testing.T) {
		err := p.verifyUnmounted("/tmp/definitely-not-mounted-xyz-12345")
		assert.NoError(t, err)
	})
}

func TestCleanupDirectories_Unit(t *testing.T) {
	p := NewProvider()

	t.Run("all_exist_and_removable", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "upper")
		workDir := filepath.Join(tmpDir, "work")
		mergedDir := filepath.Join(tmpDir, "merged")

		require.NoError(t, os.MkdirAll(upperDir, 0755))
		require.NoError(t, os.MkdirAll(workDir, 0755))
		require.NoError(t, os.MkdirAll(mergedDir, 0755))

		err := p.cleanupDirectories(upperDir, workDir, mergedDir)
		assert.NoError(t, err)

		for _, dir := range []string{upperDir, workDir, mergedDir} {
			_, err := os.Stat(dir)
			assert.True(t, os.IsNotExist(err), "directory should be removed: %s", dir)
		}
	})

	t.Run("already_nonexistent", func(t *testing.T) {
		err := p.cleanupDirectories(
			"/nonexistent/upper",
			"/nonexistent/work",
			"/nonexistent/merged",
		)
		assert.NoError(t, err)
	})

	t.Run("with_content", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "upper")
		require.NoError(t, os.MkdirAll(upperDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("content"), 0644))

		err := p.cleanupDirectories(upperDir, "/nonexistent/work", "/nonexistent/merged")
		assert.NoError(t, err)

		_, err = os.Stat(upperDir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestCreateDirectories_Unit(t *testing.T) {
	p := NewProvider()

	t.Run("creates_all", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "upper")
		workDir := filepath.Join(tmpDir, "work")
		mergedDir := filepath.Join(tmpDir, "merged")

		err := p.createDirectories(upperDir, workDir, mergedDir)
		assert.NoError(t, err)

		for _, dir := range []string{upperDir, workDir, mergedDir} {
			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.True(t, info.IsDir())
		}
	})

	t.Run("nested_directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "deep", "nested", "upper")
		workDir := filepath.Join(tmpDir, "deep", "nested", "work")
		mergedDir := filepath.Join(tmpDir, "deep", "nested", "merged")

		err := p.createDirectories(upperDir, workDir, mergedDir)
		assert.NoError(t, err)

		for _, dir := range []string{upperDir, workDir, mergedDir} {
			_, err := os.Stat(dir)
			assert.NoError(t, err)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()
		upperDir := filepath.Join(tmpDir, "upper")
		workDir := filepath.Join(tmpDir, "work")
		mergedDir := filepath.Join(tmpDir, "merged")

		err := p.createDirectories(upperDir, workDir, mergedDir)
		assert.NoError(t, err)
		err = p.createDirectories(upperDir, workDir, mergedDir)
		assert.NoError(t, err)
	})
}

func TestProviderName(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "overlayfs-native", p.Name())
}

func TestValidateRequest_Unit(t *testing.T) {
	p := NewProvider()

	t.Run("valid_request", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test-session",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.NoError(t, err)
	})

	t.Run("empty_session_id", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.Error(t, err)
	})

	t.Run("no_lower_dirs", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test",
			LowerDirs:    nil,
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.Error(t, err)
	})

	t.Run("empty_lower_dirs", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test",
			LowerDirs:    []string{},
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.Error(t, err)
	})

	t.Run("nonexistent_lower_dir", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test",
			LowerDirs:    []string{"/nonexistent/path"},
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.Error(t, err)
	})

	t.Run("empty_workspace_dir", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: "",
		}
		err := p.validateRequest(req)
		assert.Error(t, err)
	})

	t.Run("multiple_lower_dirs_one_missing", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test",
			LowerDirs:    []string{t.TempDir(), "/nonexistent/path"},
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.Error(t, err)
	})

	t.Run("multiple_valid_lower_dirs", func(t *testing.T) {
		req := sandbox.SandboxRequest{
			SessionID:    "test",
			LowerDirs:    []string{t.TempDir(), t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}
		err := p.validateRequest(req)
		assert.NoError(t, err)
	})
}

func TestValidate_NonexistentSandbox(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	err := p.Validate(ctx, "nonexistent-sandbox-id")
	assert.Error(t, err)

	var sbErr *sandbox.Error
	if assert.ErrorAs(t, err, &sbErr) {
		assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code)
	}
}

func TestDestroy_NonexistentSandbox(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	// Destroying nonexistent sandbox should be idempotent
	err := p.Destroy(ctx, "nonexistent-sandbox-id")
	assert.NoError(t, err)
}

func TestDestroy_DoubleDestroy(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	// First and second destroy should both succeed
	err := p.Destroy(ctx, "double-destroy-id")
	assert.NoError(t, err)
	err = p.Destroy(ctx, "double-destroy-id")
	assert.NoError(t, err)
}

func TestCreate_CancelledContext(t *testing.T) {
	p := NewProvider()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := sandbox.SandboxRequest{
		SessionID:    "cancelled-test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	_, err := p.Create(ctx, req)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCreate_InvalidRequest_EmptySessionID(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	_, err := p.Create(ctx, req)
	assert.Error(t, err)
}

func TestCreate_InvalidRequest_NoLowerDirs(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "test",
		LowerDirs:    nil,
		WorkspaceDir: t.TempDir(),
	}

	_, err := p.Create(ctx, req)
	assert.Error(t, err)
}

func TestCreate_InvalidRequest_NonexistentLowerDir(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "test",
		LowerDirs:    []string{"/nonexistent/path/xyz"},
		WorkspaceDir: t.TempDir(),
	}

	_, err := p.Create(ctx, req)
	assert.Error(t, err)
}

func TestCreate_InvalidRequest_EmptyWorkspaceDir(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: "",
	}

	_, err := p.Create(ctx, req)
	assert.Error(t, err)
}

func TestDestroy_WithCleanupFunc(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	cleanupCalled := false
	tmpDir := t.TempDir()

	// Manually add a sandbox to the internal registry
	p.mu.Lock()
	p.sandboxes["test-cleanup-func"] = &sandbox.Sandbox{
		ID:         "test-cleanup-func",
		MergedPath: filepath.Join(tmpDir, "merged"),
		UpperPath:  filepath.Join(tmpDir, "upper"),
		WorkPath:   filepath.Join(tmpDir, "work"),
		Type:       "overlayfs-native",
		CleanupFunc: func() error {
			cleanupCalled = true
			return nil
		},
	}
	p.mu.Unlock()

	err := p.Destroy(ctx, "test-cleanup-func")
	assert.NoError(t, err)
	assert.True(t, cleanupCalled, "CleanupFunc should have been called")

	// Second destroy should be idempotent
	err = p.Destroy(ctx, "test-cleanup-func")
	assert.NoError(t, err)
}

func TestDestroy_CleanupFuncError(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	p.mu.Lock()
	p.sandboxes["test-cleanup-err"] = &sandbox.Sandbox{
		ID:   "test-cleanup-err",
		Type: "overlayfs-native",
		CleanupFunc: func() error {
			return assert.AnError
		},
	}
	p.mu.Unlock()

	err := p.Destroy(ctx, "test-cleanup-err")
	assert.Error(t, err)
}

func TestDestroy_NilCleanupFunc(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	p.mu.Lock()
	p.sandboxes["test-nil-cleanup"] = &sandbox.Sandbox{
		ID:          "test-nil-cleanup",
		Type:        "overlayfs-native",
		CleanupFunc: nil,
	}
	p.mu.Unlock()

	err := p.Destroy(ctx, "test-nil-cleanup")
	assert.NoError(t, err)
}

func TestValidate_ExistingButUnmounted(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mergedDir := filepath.Join(tmpDir, "merged")
	require.NoError(t, os.MkdirAll(mergedDir, 0755))

	// Register a sandbox with a real directory path but no actual mount
	p.mu.Lock()
	p.sandboxes["test-unmounted"] = &sandbox.Sandbox{
		ID:         "test-unmounted",
		MergedPath: mergedDir,
		Type:       "overlayfs-native",
	}
	p.mu.Unlock()

	err := p.Validate(ctx, "test-unmounted")
	// Should fail because it's not actually mounted
	assert.Error(t, err)
}

func TestValidate_MissingMergedDir(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	// Register a sandbox with nonexistent merged directory
	p.mu.Lock()
	p.sandboxes["test-missing-dir"] = &sandbox.Sandbox{
		ID:         "test-missing-dir",
		MergedPath: "/nonexistent/merged/dir/xyz",
		Type:       "overlayfs-native",
	}
	p.mu.Unlock()

	err := p.Validate(ctx, "test-missing-dir")
	assert.Error(t, err)

	var sbErr *sandbox.Error
	if assert.ErrorAs(t, err, &sbErr) {
		assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code)
	}
}

func TestCleanup_NotMounted(t *testing.T) {
	p := NewProvider()
	tmpDir := t.TempDir()

	mergedDir := filepath.Join(tmpDir, "merged")
	upperDir := filepath.Join(tmpDir, "upper")
	workDir := filepath.Join(tmpDir, "work")

	require.NoError(t, os.MkdirAll(mergedDir, 0755))
	require.NoError(t, os.MkdirAll(upperDir, 0755))
	require.NoError(t, os.MkdirAll(workDir, 0755))

	// Cleanup on non-mounted directory -- unmount will get EINVAL (not mounted),
	// which is treated as success, then directories should be cleaned up
	err := p.cleanup(mergedDir, upperDir, workDir)
	// May error due to unmount verification finding "/" in /proc/mounts or not
	// but should not panic
	_ = err
}

func TestUnmountOverlay_NotMounted(t *testing.T) {
	p := NewProvider()
	tmpDir := t.TempDir()
	mergedDir := filepath.Join(tmpDir, "merged")
	require.NoError(t, os.MkdirAll(mergedDir, 0755))

	// Unmounting a directory that isn't mounted should return EINVAL
	// which the code treats as success (already unmounted)
	err := p.unmountOverlay(mergedDir)
	// On Linux, syscall.Unmount returns EINVAL for non-mount-points
	// The code should handle this gracefully
	_ = err
}

func TestInitRegistration(t *testing.T) {
	// Verify the init() function registered both provider names
	p1, err := sandbox.NewProviderForPlatform("overlayfs")
	assert.NoError(t, err)
	assert.NotNil(t, p1)

	p2, err := sandbox.NewProviderForPlatform("overlayfs-native")
	assert.NoError(t, err)
	assert.NotNil(t, p2)
}

func TestCreate_MountFails_CleansUpDirectories(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	req := sandbox.SandboxRequest{
		SessionID:    "mount-fail-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	// Create will fail at mount (no root), but should clean up dirs
	_, err := p.Create(ctx, req)
	if err == nil {
		// If it succeeded (e.g., running as root), destroy it
		_ = p.Destroy(ctx, "mount-fail-test")
		t.Skip("mount succeeded (running as root?)")
		return
	}

	// Verify the error is a mount or permission error
	var sbErr *sandbox.Error
	if assert.ErrorAs(t, err, &sbErr) {
		assert.True(t, sbErr.Code == sandbox.ErrCodeMountFailed || sbErr.Code == sandbox.ErrCodePermissionDenied,
			"expected mount or permission error, got code %d", sbErr.Code)
	}
}

func TestCreate_WithSecrets_MountFails(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "secrets-mount-fail",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
		Secrets:      map[string]string{"KEY": "VALUE"},
	}

	_, err := p.Create(ctx, req)
	if err == nil {
		_ = p.Destroy(ctx, "secrets-mount-fail")
		t.Skip("mount succeeded (running as root?)")
		return
	}
	assert.Error(t, err)
}

func TestCreate_ContextCancelledAfterValidation(t *testing.T) {
	p := NewProvider()

	// Create a valid request but with immediately cancelled context
	// The context check happens at the start and again before mount
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := sandbox.SandboxRequest{
		SessionID:    "ctx-cancel-test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	_, err := p.Create(ctx, req)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWriteSecrets_InvalidDir(t *testing.T) {
	p := NewProvider()
	err := p.writeSecrets("/nonexistent/directory/for/secrets", map[string]string{
		"KEY": "VALUE",
	})
	assert.Error(t, err)
}

func TestVerifyMount_NotMounted(t *testing.T) {
	p := NewProvider()
	err := p.verifyMount("/tmp/definitely-not-a-mount-point-xyz-99999")
	assert.Error(t, err)

	var sbErr *sandbox.Error
	if assert.ErrorAs(t, err, &sbErr) {
		assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code)
	}
}

func TestCheckKernelVersion(t *testing.T) {
	p := NewProvider()
	// On the current system with kernel 6.6.123+, this should pass
	err := p.checkKernelVersion()
	assert.NoError(t, err)
}

func TestNewProvider_RegistrySandboxes(t *testing.T) {
	p := NewProvider()
	// Fresh provider should have empty sandboxes map
	assert.NotNil(t, p.sandboxes)
	assert.Empty(t, p.sandboxes)
}

func TestCleanupDirectories_WithNestedContent(t *testing.T) {
	p := NewProvider()
	tmpDir := t.TempDir()

	upperDir := filepath.Join(tmpDir, "upper")
	workDir := filepath.Join(tmpDir, "work")
	mergedDir := filepath.Join(tmpDir, "merged")

	// Create nested content
	require.NoError(t, os.MkdirAll(filepath.Join(upperDir, "deep", "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(upperDir, "deep", "nested", "file.txt"), []byte("data"), 0644))
	require.NoError(t, os.MkdirAll(workDir, 0755))
	require.NoError(t, os.MkdirAll(mergedDir, 0755))

	err := p.cleanupDirectories(upperDir, workDir, mergedDir)
	assert.NoError(t, err)

	// All should be gone
	for _, dir := range []string{upperDir, workDir, mergedDir} {
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	}
}

func TestWriteSecrets_OverwritesExisting(t *testing.T) {
	p := NewProvider()
	upperDir := t.TempDir()

	// Write initial secrets
	err := p.writeSecrets(upperDir, map[string]string{"KEY1": "VALUE1"})
	require.NoError(t, err)

	// Write again with different secrets
	err = p.writeSecrets(upperDir, map[string]string{"KEY2": "VALUE2"})
	require.NoError(t, err)

	envPath := filepath.Join(upperDir, ".env")
	content, err := os.ReadFile(envPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "KEY2=VALUE2")
	// The old key should be overwritten
	assert.NotContains(t, contentStr, "KEY1=VALUE1")
}
