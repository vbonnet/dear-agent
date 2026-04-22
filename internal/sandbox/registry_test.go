package sandbox_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"

	// Import providers to trigger registration via init().
	_ "github.com/vbonnet/dear-agent/internal/sandbox/bubblewrap"
	_ "github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

// --- Provider registration and lookup ---

func TestProviderRegistration_ClaudeCodeWorktree(t *testing.T) {
	t.Parallel()

	provider, err := sandbox.NewProviderForPlatform("claudecode-worktree")
	require.NoError(t, err, "claudecode-worktree should be registered via init()")
	require.NotNil(t, provider)
	assert.Equal(t, "claudecode-worktree", provider.Name())
}

func TestProviderRegistration_OverlayFS(t *testing.T) {
	t.Parallel()

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	require.NoError(t, err, "overlayfs should be registered via init()")
	require.NotNil(t, provider)
	assert.Equal(t, "overlayfs-native", provider.Name())
}

func TestProviderRegistration_Bubblewrap(t *testing.T) {
	t.Parallel()

	provider, err := sandbox.NewProviderForPlatform("bubblewrap")
	require.NoError(t, err, "bubblewrap should be registered via init()")
	require.NotNil(t, provider)
	assert.Equal(t, "bubblewrap", provider.Name())
}

func TestProviderRegistration_Mock(t *testing.T) {
	t.Parallel()

	provider, err := sandbox.NewProviderForPlatform("mock")
	require.NoError(t, err, "mock should be a built-in provider")
	require.NotNil(t, provider)
	assert.Equal(t, "mock", provider.Name())
}

// --- Invalid/unknown provider names ---

func TestProviderLookup_UnknownName(t *testing.T) {
	t.Parallel()

	provider, err := sandbox.NewProviderForPlatform("does-not-exist")
	assert.Nil(t, provider)
	require.Error(t, err)

	var sbErr *sandbox.Error
	require.True(t, errors.As(err, &sbErr))
	assert.Equal(t, sandbox.ErrCodeUnsupportedPlatform, sbErr.Code)
}

func TestProviderLookup_EmptyName(t *testing.T) {
	t.Parallel()

	provider, err := sandbox.NewProviderForPlatform("")
	assert.Nil(t, provider)
	require.Error(t, err)

	var sbErr *sandbox.Error
	require.True(t, errors.As(err, &sbErr))
	assert.Equal(t, sandbox.ErrCodeUnsupportedPlatform, sbErr.Code)
}

// --- Default provider selection ---

func TestDefaultProviderSelection(t *testing.T) {
	t.Parallel()

	// NewProvider() should auto-detect and return a valid provider on this platform.
	provider, err := sandbox.NewProvider()
	require.NoError(t, err, "NewProvider should succeed on supported platforms")
	require.NotNil(t, provider)

	name := provider.Name()
	assert.NotEmpty(t, name)

	// On Linux, should be either bubblewrap or overlayfs-native.
	validNames := []string{"bubblewrap", "overlayfs-native", "apfs-reflink", "mock"}
	assert.Contains(t, validNames, name,
		"default provider should be one of the known providers")
}

func TestDefaultProviderMatchesPlatformRecommendation(t *testing.T) {
	t.Parallel()

	info, err := sandbox.DetectPlatform()
	require.NoError(t, err)

	provider, err := sandbox.NewProviderForPlatform(info.Recommended)
	require.NoError(t, err, "recommended provider should be available")
	require.NotNil(t, provider)
	assert.NotEmpty(t, provider.Name())
}

// --- Provider factory returns fresh instances ---

func TestProviderFactory_ReturnsDistinctInstances(t *testing.T) {
	t.Parallel()

	p1, err := sandbox.NewProviderForPlatform("claudecode-worktree")
	require.NoError(t, err)

	p2, err := sandbox.NewProviderForPlatform("claudecode-worktree")
	require.NoError(t, err)

	// Each call should return a new instance (pointer inequality).
	assert.NotSame(t, p1, p2, "factory should return distinct instances")
}

func TestProviderFactory_MockReturnsDistinctInstances(t *testing.T) {
	t.Parallel()

	p1, err := sandbox.NewProviderForPlatform("mock")
	require.NoError(t, err)

	p2, err := sandbox.NewProviderForPlatform("mock")
	require.NoError(t, err)

	assert.NotSame(t, p1, p2, "mock factory should return distinct instances")
}

// --- Multiple unimplemented providers ---

func TestProviderLookup_MultipleUnimplemented(t *testing.T) {
	t.Parallel()

	unimplemented := []string{
		"fuse-overlayfs",
		"macfuse",
		"fallback",
		"docker",
		"nsjail",
	}

	for _, name := range unimplemented {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			provider, err := sandbox.NewProviderForPlatform(name)
			assert.Nil(t, provider, "unimplemented provider %q should return nil", name)
			require.Error(t, err)

			var sbErr *sandbox.Error
			require.True(t, errors.As(err, &sbErr),
				"error should be *sandbox.Error for %q", name)
			assert.Equal(t, sandbox.ErrCodeUnsupportedPlatform, sbErr.Code)
		})
	}
}
