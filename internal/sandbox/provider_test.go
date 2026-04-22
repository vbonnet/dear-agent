package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// testProvider runs standard contract tests against any Provider implementation.
// Call this from each provider's test file:
//
//	func TestOverlayFSProvider(t *testing.T) {
//	    provider := overlayfs.NewProvider()
//	    testProvider(t, provider)
//	}
func testProvider(t *testing.T, provider sandbox.Provider) {
	t.Run("Create_and_Destroy", func(t *testing.T) {
		t.Parallel()
		testCreateAndDestroy(t, provider)
	})

	t.Run("Create_with_timeout", func(t *testing.T) {
		t.Parallel()
		testCreateWithTimeout(t, provider)
	})

	t.Run("Destroy_idempotent", func(t *testing.T) {
		t.Parallel()
		testDestroyIdempotent(t, provider)
	})

	t.Run("Validate_nonexistent", func(t *testing.T) {
		t.Parallel()
		testValidateNonexistent(t, provider)
	})
}

func testCreateAndDestroy(t *testing.T, provider sandbox.Provider) {
	ctx := context.Background()

	req := sandbox.SandboxRequest{
		SessionID:    "test-session-123",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
		Secrets:      map[string]string{"TEST_KEY": "test_value"},
	}

	// Create sandbox
	sb, err := provider.Create(ctx, req)
	require.NoError(t, err, "Create should succeed")
	require.NotNil(t, sb, "Sandbox should not be nil")

	// Verify sandbox structure
	assert.Equal(t, req.SessionID, sb.ID, "ID should match request")
	assert.NotEmpty(t, sb.MergedPath, "MergedPath should not be empty")
	assert.NotEmpty(t, sb.UpperPath, "UpperPath should not be empty")
	assert.NotEmpty(t, sb.WorkPath, "WorkPath should not be empty")
	assert.NotEmpty(t, sb.Type, "Type should not be empty")
	assert.False(t, sb.CreatedAt.IsZero(), "CreatedAt should be set")

	// Validate sandbox exists
	err = provider.Validate(ctx, sb.ID)
	assert.NoError(t, err, "Validate should succeed for existing sandbox")

	// Destroy sandbox
	err = provider.Destroy(ctx, sb.ID)
	require.NoError(t, err, "Destroy should succeed")

	// Validate sandbox is gone
	err = provider.Validate(ctx, sb.ID)
	assert.Error(t, err, "Validate should fail after Destroy")

	// Verify error type is ErrCodeSandboxNotFound
	var sbErr *sandbox.Error
	if errors.As(err, &sbErr) {
		assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code,
			"Error code should be ErrCodeSandboxNotFound")
	}
}

func testCreateWithTimeout(t *testing.T, provider sandbox.Provider) {
	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := sandbox.SandboxRequest{
		SessionID:    "timeout-test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	// Should fail due to cancelled context
	_, err := provider.Create(ctx, req)
	assert.Error(t, err, "Create should fail with cancelled context")

	// Error should be context-related
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
		"Error should be context.Canceled or context.DeadlineExceeded")
}

func testDestroyIdempotent(t *testing.T, provider sandbox.Provider) {
	ctx := context.Background()

	// Destroy non-existent sandbox (should not error)
	err := provider.Destroy(ctx, "nonexistent-sandbox-xyz")
	assert.NoError(t, err, "Destroy should be idempotent for non-existent sandbox")
}

func testValidateNonexistent(t *testing.T, provider sandbox.Provider) {
	ctx := context.Background()

	err := provider.Validate(ctx, "nonexistent-sandbox-xyz")
	require.Error(t, err, "Validate should fail for non-existent sandbox")

	// Check it's the right error type
	var sbErr *sandbox.Error
	if assert.True(t, errors.As(err, &sbErr), "Error should be *sandbox.Error") {
		assert.Equal(t, sandbox.ErrCodeSandboxNotFound, sbErr.Code,
			"Error code should be ErrCodeSandboxNotFound")
	}
}

// TestMockProvider verifies the MockProvider satisfies the contract
func TestMockProvider(t *testing.T) {
	provider := sandbox.NewMockProvider()
	require.NotNil(t, provider, "NewMockProvider should return non-nil")

	// Run standard contract tests
	testProvider(t, provider)
}

// TestMockProviderErrorInjection tests error injection capabilities
func TestMockProviderErrorInjection(t *testing.T) {
	t.Run("Create_error_injection", func(t *testing.T) {
		t.Parallel()
		provider := sandbox.NewMockProvider()
		expectedErr := sandbox.NewError(sandbox.ErrCodeMountFailed, "injected error")
		provider.SetCreateError(expectedErr)

		ctx := context.Background()
		req := sandbox.SandboxRequest{
			SessionID:    "test-error",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}

		_, err := provider.Create(ctx, req)
		require.Error(t, err, "Create should fail with injected error")
		assert.Equal(t, expectedErr, err, "Error should match injected error")
	})

	t.Run("Destroy_error_injection", func(t *testing.T) {
		t.Parallel()
		provider := sandbox.NewMockProvider()
		ctx := context.Background()

		// Create a sandbox first
		req := sandbox.SandboxRequest{
			SessionID:    "test-destroy-error",
			LowerDirs:    []string{t.TempDir()},
			WorkspaceDir: t.TempDir(),
		}
		sb, err := provider.Create(ctx, req)
		require.NoError(t, err)

		// Inject destroy error
		expectedErr := sandbox.NewError(sandbox.ErrCodeUnmountFailed, "injected destroy error")
		provider.SetDestroyError(expectedErr)

		err = provider.Destroy(ctx, sb.ID)
		require.Error(t, err, "Destroy should fail with injected error")
		assert.Equal(t, expectedErr, err, "Error should match injected error")
	})
}

// TestMockProviderConcurrency verifies thread-safety
func TestMockProviderConcurrency(t *testing.T) {
	provider := sandbox.NewMockProvider()
	ctx := context.Background()

	// Create multiple sandboxes concurrently
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			req := sandbox.SandboxRequest{
				SessionID:    string(rune('A' + id)),
				LowerDirs:    []string{t.TempDir()},
				WorkspaceDir: t.TempDir(),
			}
			_, err := provider.Create(ctx, req)
			results <- err
		}(i)
	}

	// Verify all succeeded
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(t, err, "Concurrent Create should succeed")
	}
}
