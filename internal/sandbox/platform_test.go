package sandbox_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"

	// Import providers to trigger registration
	_ "github.com/vbonnet/dear-agent/internal/sandbox/bubblewrap"
	_ "github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

func TestPlatformDetection(t *testing.T) {
	info, err := sandbox.DetectPlatform()
	require.NoError(t, err)

	t.Logf("Platform: %s", info.OS)
	t.Logf("Kernel: %s", info.KernelVersion)
	t.Logf("OverlayFS: %v", info.HasOverlayFS)
	t.Logf("APFS: %v", info.HasAPFS)
	t.Logf("Recommended: %s", info.Recommended)

	// Verify OS detected correctly
	assert.Equal(t, runtime.GOOS, info.OS)

	// Platform-specific checks
	switch runtime.GOOS {
	case "linux":
		assert.NotEmpty(t, info.KernelVersion)
		assert.False(t, info.HasAPFS)
	case "darwin":
		assert.True(t, info.HasAPFS)
		assert.False(t, info.HasOverlayFS)
	}
}

func TestProviderAvailability(t *testing.T) {
	info, err := sandbox.DetectPlatform()
	require.NoError(t, err)

	// Try to create recommended provider
	provider, err := sandbox.NewProviderForPlatform(info.Recommended)

	switch runtime.GOOS {
	case "linux":
		require.NoError(t, err, "Provider should be available on Linux")
		// Accept bubblewrap or overlayfs-native depending on what's installed
		assert.Contains(t, []string{"bubblewrap", "overlayfs-native"}, provider.Name())
		t.Logf("Recommended provider: %s", provider.Name())
	case "darwin":
		// APFS provider not yet implemented, so expect error
		if err != nil {
			t.Logf("APFS provider not yet implemented: %v", err)
		} else {
			assert.Equal(t, "apfs-reflink", provider.Name())
		}
	}
}

func BenchmarkSandboxCreation(b *testing.B) {
	provider := getMockOrRealProvider(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    fmt.Sprintf("bench-%d", i),
			LowerDirs:    []string{b.TempDir()},
			WorkspaceDir: b.TempDir(),
		})
		if err != nil {
			b.Fatal(err)
		}
		_ = provider.Destroy(ctx, sb.ID)
	}
}

func BenchmarkConcurrentSandboxes(b *testing.B) {
	provider := getMockOrRealProvider(b)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ctx := context.Background()
			sb, err := provider.Create(ctx, sandbox.SandboxRequest{
				SessionID:    fmt.Sprintf("concurrent-%d-%d", time.Now().UnixNano(), i),
				LowerDirs:    []string{b.TempDir()},
				WorkspaceDir: b.TempDir(),
			})
			if err != nil {
				b.Fatal(err)
			}
			_ = provider.Destroy(ctx, sb.ID)
			i++
		}
	})
}

func BenchmarkSandboxCreationWithSecrets(b *testing.B) {
	provider := getMockOrRealProvider(b)

	secrets := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-test-key",
		"GITHUB_TOKEN":      "ghp_test_token",
		"DATABASE_URL":      "postgresql://test:test@localhost/testdb",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    fmt.Sprintf("bench-secrets-%d", i),
			LowerDirs:    []string{b.TempDir()},
			WorkspaceDir: b.TempDir(),
			Secrets:      secrets,
		})
		if err != nil {
			b.Fatal(err)
		}
		_ = provider.Destroy(ctx, sb.ID)
	}
}

func BenchmarkSandboxValidation(b *testing.B) {
	provider := getMockOrRealProvider(b)
	ctx := context.Background()

	// Create a sandbox for validation
	sb, err := provider.Create(ctx, sandbox.SandboxRequest{
		SessionID:    "bench-validate",
		LowerDirs:    []string{b.TempDir()},
		WorkspaceDir: b.TempDir(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer provider.Destroy(ctx, sb.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := provider.Validate(ctx, sb.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func getMockOrRealProvider(t testing.TB) sandbox.Provider {
	// Use real provider if available, otherwise mock
	provider, err := sandbox.NewProvider()
	if err != nil {
		t.Logf("Real provider not available, using mock: %v", err)
		return sandbox.NewMockProvider()
	}

	// Test if provider actually works by attempting a quick create/destroy
	ctx := context.Background()
	testSb, err := provider.Create(ctx, sandbox.SandboxRequest{
		SessionID:    "provider-test",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	})
	if err != nil {
		t.Logf("Real provider create failed, using mock: %v", err)
		return sandbox.NewMockProvider()
	}
	_ = provider.Destroy(ctx, testSb.ID)

	return provider
}
