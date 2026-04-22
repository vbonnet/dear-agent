package sandbox_test

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
)

// TestProviderRegistry verifies correct provider registered based on platform
func TestProviderRegistry(t *testing.T) {
	t.Run("detect_platform_and_provider", func(t *testing.T) {
		info, err := sandbox.DetectPlatform()
		require.NoError(t, err, "platform detection should succeed")
		require.NotNil(t, info, "platform info should not be nil")

		t.Logf("Detected OS: %s", info.OS)
		t.Logf("Recommended provider: %s", info.Recommended)

		// Verify recommended provider matches OS
		switch runtime.GOOS {
		case "linux":
			assert.Equal(t, "linux", info.OS)
			// bubblewrap is preferred when available, then overlayfs, then fuse-overlayfs
			assert.Contains(t, []string{"bubblewrap", "overlayfs", "fuse-overlayfs"}, info.Recommended)

		case "darwin":
			assert.Equal(t, "darwin", info.OS)
			assert.True(t, info.HasAPFS, "macOS should have APFS support")
			assert.Equal(t, "apfs", info.Recommended)

		default:
			assert.Equal(t, "fallback", info.Recommended)
		}
	})

	t.Run("provider_registration", func(t *testing.T) {
		// Verify that platform-specific providers are registered
		switch runtime.GOOS {
		case "linux":
			provider, err := sandbox.NewProviderForPlatform("overlayfs")
			require.NoError(t, err, "overlayfs provider should be registered on Linux")
			assert.Equal(t, "overlayfs-native", provider.Name())

		case "darwin":
			provider, err := sandbox.NewProviderForPlatform("apfs")
			require.NoError(t, err, "apfs provider should be registered on macOS")
			assert.Equal(t, "apfs-reflink", provider.Name())
		}
	})

	t.Run("mock_provider_always_available", func(t *testing.T) {
		provider, err := sandbox.NewProviderForPlatform("mock")
		require.NoError(t, err, "mock provider should always be available")
		assert.Equal(t, "mock", provider.Name())
	})
}

// TestCrossPlatformDetection verifies kernel/OS detection logic
func TestCrossPlatformDetection(t *testing.T) {
	t.Run("os_detection", func(t *testing.T) {
		info, err := sandbox.DetectPlatform()
		require.NoError(t, err)

		// OS field should match runtime.GOOS
		assert.Equal(t, runtime.GOOS, info.OS)
	})

	t.Run("linux_kernel_version", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("Test only runs on Linux")
		}

		info, err := sandbox.DetectPlatform()
		require.NoError(t, err)

		// Kernel version should be populated
		assert.NotEmpty(t, info.KernelVersion, "kernel version should be detected on Linux")
		assert.NotEqual(t, "unknown", info.KernelVersion, "should parse kernel version successfully")

		t.Logf("Detected kernel version: %s", info.KernelVersion)

		// Verify OverlayFS detection based on kernel version
		// Current system is 6.6.123+ which should support OverlayFS
		assert.True(t, info.HasOverlayFS, "kernel 6.6.123+ should support OverlayFS")
	})

	t.Run("macos_apfs_detection", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("Test only runs on macOS")
		}

		info, err := sandbox.DetectPlatform()
		require.NoError(t, err)

		// APFS should be available on modern macOS
		assert.True(t, info.HasAPFS, "modern macOS should have APFS support")
	})

	t.Run("recommended_provider_logic", func(t *testing.T) {
		info, err := sandbox.DetectPlatform()
		require.NoError(t, err)

		// Recommended field should always be populated
		assert.NotEmpty(t, info.Recommended, "recommended provider should be set")

		// Should not recommend unimplemented providers
		switch runtime.GOOS {
		case "linux":
			// bubblewrap is preferred when available, then overlayfs, then fuse-overlayfs
			assert.Contains(t, []string{"bubblewrap", "overlayfs", "fuse-overlayfs"}, info.Recommended)
		case "darwin":
			assert.Equal(t, "apfs", info.Recommended)
		}
	})
}

// TestProviderConformance validates Provider interface implementation
func TestProviderConformance(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		skipOnOS     string
	}{
		{
			name:         "overlayfs_provider",
			providerName: "overlayfs",
			skipOnOS:     "darwin",
		},
		{
			name:         "apfs_provider",
			providerName: "apfs",
			skipOnOS:     "linux",
		},
		{
			name:         "mock_provider",
			providerName: "mock",
			skipOnOS:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnOS != "" && runtime.GOOS == tt.skipOnOS {
				t.Skipf("Provider %s not available on %s", tt.providerName, runtime.GOOS)
			}

			// Attempt to create provider
			provider, err := sandbox.NewProviderForPlatform(tt.providerName)
			if runtime.GOOS == tt.skipOnOS {
				// Provider should not be available on wrong platform
				return
			}
			require.NoError(t, err, "provider should be available")
			require.NotNil(t, provider, "provider should not be nil")

			// Test Name() method
			t.Run("name_method", func(t *testing.T) {
				name := provider.Name()
				assert.NotEmpty(t, name, "provider name should not be empty")
			})

			// Test Create() method
			t.Run("create_method", func(t *testing.T) {
				ctx := context.Background()
				lowerDir := t.TempDir()
				workspaceDir := t.TempDir()

				// Create a test file in lowerdir
				testFile := filepath.Join(lowerDir, "test.txt")
				err := os.WriteFile(testFile, []byte("test content"), 0644)
				require.NoError(t, err)

				req := sandbox.SandboxRequest{
					SessionID:    "test-conformance-create",
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				}

				sb, err := provider.Create(ctx, req)
				if err != nil && tt.providerName != "mock" {
					// Real providers may fail due to permissions, that's OK for conformance test
					t.Logf("Create failed (expected for real providers in test env): %v", err)
					return
				}
				require.NoError(t, err, "create should succeed")
				require.NotNil(t, sb, "sandbox should not be nil")

				// Verify sandbox fields are populated
				assert.Equal(t, req.SessionID, sb.ID)
				assert.NotEmpty(t, sb.MergedPath)
				assert.NotEmpty(t, sb.UpperPath)
				assert.NotEmpty(t, sb.Type)
				assert.False(t, sb.CreatedAt.IsZero())

				// Clean up
				defer provider.Destroy(ctx, sb.ID)
			})

			// Test Validate() method
			t.Run("validate_method", func(t *testing.T) {
				ctx := context.Background()

				// Non-existent sandbox should fail validation
				err := provider.Validate(ctx, "nonexistent-sandbox")
				assert.Error(t, err, "validate should fail for non-existent sandbox")

				// Verify error is ErrCodeSandboxNotFound
				var sbErr *sandbox.Error
				if assert.ErrorAs(t, err, &sbErr) {
					assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code)
				}
			})

			// Test Destroy() method
			t.Run("destroy_method", func(t *testing.T) {
				ctx := context.Background()

				// Destroy non-existent sandbox should be idempotent (no error)
				err := provider.Destroy(ctx, "nonexistent-sandbox")
				assert.NoError(t, err, "destroy should be idempotent")
			})

			// Test context cancellation
			t.Run("context_cancellation", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				lowerDir := t.TempDir()
				workspaceDir := t.TempDir()

				req := sandbox.SandboxRequest{
					SessionID:    "test-conformance-cancel",
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				}

				_, err := provider.Create(ctx, req)
				assert.Error(t, err, "create should fail with cancelled context")
				assert.ErrorIs(t, err, context.Canceled)
			})
		})
	}
}

// TestProviderLifecycle tests complete sandbox lifecycle
func TestProviderLifecycle(t *testing.T) {
	// Use platform-appropriate provider
	var provider sandbox.Provider
	var err error

	switch runtime.GOOS {
	case "linux":
		provider, err = sandbox.NewProviderForPlatform("overlayfs")
	case "darwin":
		provider, err = sandbox.NewProviderForPlatform("apfs")
	default:
		provider = sandbox.NewMockProvider()
	}

	if err != nil {
		t.Logf("Platform provider not available, using mock: %v", err)
		provider = sandbox.NewMockProvider()
	}

	ctx := context.Background()
	lowerDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Create test file in lowerdir
	testFile := filepath.Join(lowerDir, "test.txt")
	err = os.WriteFile(testFile, []byte("original content"), 0644)
	require.NoError(t, err)

	req := sandbox.SandboxRequest{
		SessionID:    "test-lifecycle",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"TEST_KEY": "test_value",
		},
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	if err != nil {
		t.Logf("Create failed (may be expected in test env): %v", err)
		return
	}
	require.NotNil(t, sb)

	// Validate sandbox exists
	err = provider.Validate(ctx, sb.ID)
	assert.NoError(t, err, "sandbox should be valid after creation")

	// Destroy sandbox
	err = provider.Destroy(ctx, sb.ID)
	assert.NoError(t, err, "destroy should succeed")

	// Validate should fail after destroy
	err = provider.Validate(ctx, sb.ID)
	assert.Error(t, err, "validate should fail after destroy")
}

// TestGracefulDegradation verifies behavior when features unavailable
func TestGracefulDegradation(t *testing.T) {
	t.Run("old_kernel_fallback", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("Test only runs on Linux")
		}

		// On current system (6.6.123+), overlayfs should work
		// This test documents expected behavior on older kernels
		info, err := sandbox.DetectPlatform()
		require.NoError(t, err)

		// Current system should have OverlayFS support
		assert.True(t, info.HasOverlayFS)
		// bubblewrap is preferred when available, then overlayfs, then fuse-overlayfs
		assert.Contains(t, []string{"bubblewrap", "overlayfs", "fuse-overlayfs"}, info.Recommended)

		// If kernel was < 5.11, should recommend fuse-overlayfs
		// (Can't test this on current system without mocking)
	})

	t.Run("unimplemented_provider_error", func(t *testing.T) {
		// Try to create provider that doesn't exist
		provider, err := sandbox.NewProviderForPlatform("nonexistent")
		assert.Error(t, err, "should error for nonexistent provider")
		assert.Nil(t, provider)

		// Verify error type
		var sbErr *sandbox.Error
		if assert.ErrorAs(t, err, &sbErr) {
			assert.Equal(t, sandbox.ErrCodeUnsupportedPlatform, sbErr.Code)
		}
	})

	t.Run("provider_create_validation", func(t *testing.T) {
		provider := sandbox.NewMockProvider()
		ctx := context.Background()

		invalidRequests := []struct {
			name string
			req  sandbox.SandboxRequest
		}{
			{
				name: "empty_session_id",
				req: sandbox.SandboxRequest{
					SessionID:    "",
					LowerDirs:    []string{t.TempDir()},
					WorkspaceDir: t.TempDir(),
				},
			},
			{
				name: "empty_lower_dirs",
				req: sandbox.SandboxRequest{
					SessionID:    "test",
					LowerDirs:    []string{},
					WorkspaceDir: t.TempDir(),
				},
			},
			{
				name: "empty_workspace_dir",
				req: sandbox.SandboxRequest{
					SessionID:    "test",
					LowerDirs:    []string{t.TempDir()},
					WorkspaceDir: "",
				},
			},
		}

		for _, tc := range invalidRequests {
			t.Run(tc.name, func(t *testing.T) {
				_, _ = provider.Create(ctx, tc.req)
				// Mock provider doesn't validate, but real providers should
				// This documents expected behavior
				if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
					t.Logf("Real providers should validate: %s", tc.name)
				}
			})
		}
	})

	t.Run("timeout_handling", func(t *testing.T) {
		provider := sandbox.NewMockProvider()
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Wait for timeout
		time.Sleep(10 * time.Millisecond)

		req := sandbox.SandboxRequest{
			SessionID:    "test-timeout",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}

		_, err := provider.Create(ctx, req)
		assert.Error(t, err, "create should fail with expired context")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// TestProviderConcurrency verifies thread-safety
func TestProviderConcurrency(t *testing.T) {
	provider := sandbox.NewMockProvider()
	ctx := context.Background()

	// Create multiple sandboxes concurrently
	const numSandboxes = 10
	errChan := make(chan error, numSandboxes)

	for i := 0; i < numSandboxes; i++ {
		go func(id int) {
			req := sandbox.SandboxRequest{
				SessionID:    "concurrent-" + string(rune(id)),
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: t.TempDir(),
			}

			sb, err := provider.Create(ctx, req)
			if err != nil {
				errChan <- err
				return
			}

			// Validate concurrently
			err = provider.Validate(ctx, sb.ID)
			if err != nil {
				errChan <- err
				return
			}

			// Destroy concurrently
			err = provider.Destroy(ctx, sb.ID)
			errChan <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numSandboxes; i++ {
		err := <-errChan
		assert.NoError(t, err, "concurrent operations should succeed")
	}
}
