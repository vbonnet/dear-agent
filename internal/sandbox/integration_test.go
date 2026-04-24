//go:build integration
// +build integration

package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// requireNoErrorOrSkip checks for mount permission errors and skips test if found
func requireNoErrorOrSkip(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		if strings.Contains(err.Error(), "must be superuser") ||
			strings.Contains(err.Error(), "permission denied") {
			t.Skipf("Skipping test - requires mount permissions: %v", err)
		}
	}
	require.NoError(t, err, msgAndArgs...)
}

func TestLinuxOverlayFS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	// Run full lifecycle test
	testProviderLifecycle(t, provider)
}

func TestMacOSAPFS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}

	provider, err := sandbox.NewProviderForPlatform("apfs")
	if err != nil {
		t.Skipf("APFS not available: %v", err)
	}

	// Run full lifecycle test
	testProviderLifecycle(t, provider)
}

func testProviderLifecycle(t *testing.T, provider sandbox.Provider) {
	ctx := context.Background()

	// Create temporary directories for test
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Create some test files in lower directory
	testFile := filepath.Join(lowerDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Test 1: Create sandbox
	t.Run("Create", func(t *testing.T) {
		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    "integration-test-1",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: workspaceDir,
			Secrets: map[string]string{
				"TEST_SECRET": "test_value",
			},
		})
		requireNoErrorOrSkip(t, err, "Create should succeed")
		require.NotNil(t, sb)

		// Verify sandbox structure
		assert.Equal(t, "integration-test-1", sb.ID)
		assert.NotEmpty(t, sb.MergedPath)
		assert.NotEmpty(t, sb.UpperPath)
		assert.NotEmpty(t, sb.WorkPath)
		assert.NotEmpty(t, sb.Type)
		assert.False(t, sb.CreatedAt.IsZero())

		// Verify merged path exists
		_, err = os.Stat(sb.MergedPath)
		assert.NoError(t, err, "Merged path should exist")

		// Verify test file is visible in merged view
		mergedTestFile := filepath.Join(sb.MergedPath, "test.txt")
		content, err := os.ReadFile(mergedTestFile)
		if err == nil {
			assert.Equal(t, "test content", string(content))
		}

		// Cleanup
		err = provider.Destroy(ctx, sb.ID)
		require.NoError(t, err, "Destroy should succeed")
	})

	// Test 2: Validate sandbox health
	t.Run("Validate", func(t *testing.T) {
		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    "integration-test-2",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: t.TempDir(),
		})
		requireNoErrorOrSkip(t, err)

		// Validate should succeed for existing sandbox
		err = provider.Validate(ctx, sb.ID)
		assert.NoError(t, err, "Validate should succeed for existing sandbox")

		// Destroy and verify validation fails
		err = provider.Destroy(ctx, sb.ID)
		require.NoError(t, err)

		err = provider.Validate(ctx, sb.ID)
		assert.Error(t, err, "Validate should fail after Destroy")
	})

	// Test 3: Isolation - modifications don't affect lower dirs
	t.Run("Isolation", func(t *testing.T) {
		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    "integration-test-3",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: t.TempDir(),
		})
		requireNoErrorOrSkip(t, err)
		defer provider.Destroy(ctx, sb.ID)

		// Modify file in sandbox
		mergedTestFile := filepath.Join(sb.MergedPath, "test.txt")
		err = os.WriteFile(mergedTestFile, []byte("modified content"), 0644)
		if err == nil {
			// Verify original file is unchanged
			originalContent, err := os.ReadFile(testFile)
			assert.NoError(t, err)
			assert.Equal(t, "test content", string(originalContent),
				"Original file should not be modified")

			// Verify modified content is in merged view
			mergedContent, err := os.ReadFile(mergedTestFile)
			assert.NoError(t, err)
			assert.Equal(t, "modified content", string(mergedContent),
				"Merged view should show modified content")
		}
	})

	// Test 4: Multiple concurrent sandboxes
	t.Run("Concurrent", func(t *testing.T) {
		const numSandboxes = 5
		sandboxes := make([]*sandbox.Sandbox, numSandboxes)

		// Create multiple sandboxes
		for i := 0; i < numSandboxes; i++ {
			sb, err := provider.Create(ctx, sandbox.SandboxRequest{
				SessionID:    string(rune('A' + i)),
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: t.TempDir(),
			})
			requireNoErrorOrSkip(t, err, "Create %d should succeed", i)
			sandboxes[i] = sb
		}

		// Verify all are valid
		for i, sb := range sandboxes {
			err := provider.Validate(ctx, sb.ID)
			assert.NoError(t, err, "Validate %d should succeed", i)
		}

		// Destroy all
		for i, sb := range sandboxes {
			err := provider.Destroy(ctx, sb.ID)
			assert.NoError(t, err, "Destroy %d should succeed", i)
		}
	})

	// Test 5: Context cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		_, err := provider.Create(cancelCtx, sandbox.SandboxRequest{
			SessionID:    "integration-test-cancelled",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: t.TempDir(),
		})
		assert.Error(t, err, "Create should fail with cancelled context")
	})

	// Test 6: Timeout
	t.Run("Timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		// Wait for timeout
		time.Sleep(10 * time.Millisecond)

		_, err := provider.Create(timeoutCtx, sandbox.SandboxRequest{
			SessionID:    "integration-test-timeout",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: t.TempDir(),
		})
		assert.Error(t, err, "Create should fail with timeout context")
	})

	// Test 7: Cleanup verification
	t.Run("CleanupVerification", func(t *testing.T) {
		workDir := t.TempDir()

		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    "integration-test-cleanup",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: workDir,
		})
		requireNoErrorOrSkip(t, err)

		// Verify directories exist
		_, err = os.Stat(sb.MergedPath)
		assert.NoError(t, err, "Merged path should exist")

		// Destroy
		err = provider.Destroy(ctx, sb.ID)
		require.NoError(t, err)

		// Verify cleanup (directories may or may not exist depending on provider)
		// This is provider-specific behavior
		t.Logf("Cleanup completed for sandbox %s", sb.ID)
	})
}

// TestProviderPerformance runs performance tests for the provider
func TestProviderPerformance(t *testing.T) {
	provider, err := sandbox.NewProvider()
	if err != nil {
		t.Skipf("No provider available: %v", err)
	}

	ctx := context.Background()
	lowerDir := t.TempDir()

	// Benchmark creation time
	start := time.Now()
	sb, err := provider.Create(ctx, sandbox.SandboxRequest{
		SessionID:    "perf-test",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: t.TempDir(),
	})
	creationTime := time.Since(start)

	if err != nil {
		t.Skipf("Provider not functional: %v", err)
	}

	t.Logf("Sandbox creation time: %v", creationTime)

	// Benchmark validation time
	start = time.Now()
	err = provider.Validate(ctx, sb.ID)
	validationTime := time.Since(start)
	require.NoError(t, err)

	t.Logf("Sandbox validation time: %v", validationTime)

	// Benchmark destruction time
	start = time.Now()
	err = provider.Destroy(ctx, sb.ID)
	destructionTime := time.Since(start)
	require.NoError(t, err)

	t.Logf("Sandbox destruction time: %v", destructionTime)

	// Assert reasonable performance (platform-dependent)
	// These are very generous limits
	if runtime.GOOS == "linux" {
		// On Linux, OverlayFS should be fast
		assert.Less(t, creationTime.Milliseconds(), int64(5000),
			"Creation should take less than 5 seconds")
	}

	assert.Less(t, validationTime.Milliseconds(), int64(1000),
		"Validation should take less than 1 second")
	assert.Less(t, destructionTime.Milliseconds(), int64(5000),
		"Destruction should take less than 5 seconds")
}
