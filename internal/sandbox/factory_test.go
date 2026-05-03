package sandbox_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/vbonnet/dear-agent/internal/sandbox"

	// Import providers to trigger registration
	_ "github.com/vbonnet/dear-agent/internal/sandbox/bubblewrap"
	_ "github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

func TestDetectPlatform(t *testing.T) {
	info, err := sandbox.DetectPlatform()
	if err != nil {
		t.Fatalf("DetectPlatform failed: %v", err)
	}

	t.Logf("Detected platform: %+v", info)

	// Basic sanity checks
	if info.OS == "" {
		t.Error("OS is empty")
	}

	if info.Recommended == "" {
		t.Error("Recommended provider is empty")
	}

	// OS-specific checks
	if info.OS == "linux" {
		if info.KernelVersion == "" {
			t.Error("KernelVersion should be set on Linux")
		}
		t.Logf("Linux kernel: %s, OverlayFS: %v", info.KernelVersion, info.HasOverlayFS)

		// On current system (6.6.123+), OverlayFS should be supported
		if runtime.GOOS == "linux" {
			if !info.HasOverlayFS {
				t.Error("HasOverlayFS should be true on kernel 6.6.123+")
			}
			// Priority: bubblewrap (works everywhere) > overlayfs (requires caps) > fuse-overlayfs
			// If bubblewrap is installed, it should be recommended
			if info.Recommended != "bubblewrap" && info.Recommended != "overlayfs" {
				t.Errorf("Recommended should be 'bubblewrap' or 'overlayfs', got '%s'", info.Recommended)
			}
			t.Logf("Recommended provider: %s", info.Recommended)
		}
	}

	if info.OS == "darwin" {
		if !info.HasAPFS {
			t.Error("HasAPFS should be true on macOS")
		}
		if info.Recommended != "apfs" {
			t.Errorf("Recommended should be 'apfs', got '%s'", info.Recommended)
		}
	}
}

func TestNewMockProvider(t *testing.T) {
	provider, err := sandbox.NewProviderForPlatform("mock")
	if err != nil {
		t.Fatalf("NewProviderForPlatform(mock) failed: %v", err)
	}

	if provider == nil {
		t.Fatal("Provider is nil")
	}

	if provider.Name() != "mock" {
		t.Errorf("Name() = %s, want mock", provider.Name())
	}
}

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
			name:     "kernel_with_dash",
			input:    "Linux version 5.11.0-ubuntu1 (builder@host) #1 SMP",
			expected: "5.11.0",
		},
		{
			name:     "minimal_version",
			input:    "Linux version 5.4.0",
			expected: "5.4.0",
		},
		{
			name:     "no_version_keyword",
			input:    "Some random string without version",
			expected: "unknown",
		},
		{
			name:     "version_at_end",
			input:    "Linux version",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Access the unexported function through DetectPlatform behavior
			// We'll test this indirectly by checking the actual kernel version
			// For direct testing, we'd need to export parseKernelVersion or use a test helper

			// Instead, verify the function works on real /proc/version
			if tt.name == "standard_kernel_version" && runtime.GOOS == "linux" {
				info, err := sandbox.DetectPlatform()
				if err != nil {
					t.Fatalf("DetectPlatform failed: %v", err)
				}

				// Verify we got a valid version (not "unknown")
				if info.KernelVersion == "unknown" {
					t.Error("KernelVersion should not be 'unknown' on Linux")
				}

				t.Logf("Detected kernel version: %s", info.KernelVersion)
			}
		})
	}
}

func TestIsKernelVersionAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		major    int
		minor    int
		expected bool
	}{
		{
			name:     "exact_match",
			version:  "5.11.0",
			major:    5,
			minor:    11,
			expected: true,
		},
		{
			name:     "higher_major",
			version:  "6.6.123",
			major:    5,
			minor:    11,
			expected: true,
		},
		{
			name:     "higher_minor",
			version:  "5.15.0",
			major:    5,
			minor:    11,
			expected: true,
		},
		{
			name:     "lower_minor",
			version:  "5.10.0",
			major:    5,
			minor:    11,
			expected: false,
		},
		{
			name:     "lower_major",
			version:  "4.19.0",
			major:    5,
			minor:    11,
			expected: false,
		},
		{
			name:     "invalid_version",
			version:  "unknown",
			major:    5,
			minor:    11,
			expected: false,
		},
		{
			name:     "malformed_version",
			version:  "not.a.version",
			major:    5,
			minor:    11,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since isKernelVersionAtLeast is unexported, we test it through DetectPlatform
			// For a real implementation, we'd want to either:
			// 1. Export the function for testing
			// 2. Use a test helper
			// 3. Test indirectly through the public API

			// For now, verify the current system detection works correctly
			if runtime.GOOS == "linux" && tt.name == "higher_major" {
				info, err := sandbox.DetectPlatform()
				if err != nil {
					t.Fatalf("DetectPlatform failed: %v", err)
				}

				// Kernel 6.6.123 should satisfy >= 5.11
				if !info.HasOverlayFS {
					t.Error("HasOverlayFS should be true for kernel >= 5.11")
				}
			}
		})
	}
}

func TestNewProviderForPlatform_Implemented(t *testing.T) {
	t.Run("overlayfs", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skipf("overlayfs is Linux-only; skipping on %s", runtime.GOOS)
		}
		provider, err := sandbox.NewProviderForPlatform("overlayfs")

		if err != nil {
			t.Errorf("NewProviderForPlatform(overlayfs) should succeed: %v", err)
		}

		if provider == nil {
			t.Error("Provider should not be nil for overlayfs")
		}

		if provider != nil && provider.Name() != "overlayfs-native" {
			t.Errorf("Provider name should be 'overlayfs-native', got '%s'", provider.Name())
		}
	})

	t.Run("apfs", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skipf("apfs is Darwin-only; skipping on %s", runtime.GOOS)
		}
		provider, err := sandbox.NewProviderForPlatform("apfs")
		if err != nil {
			t.Errorf("NewProviderForPlatform(apfs) should succeed on darwin: %v", err)
		}
		if provider == nil {
			t.Error("Provider should not be nil for apfs")
		}
	})

	t.Run("bubblewrap", func(t *testing.T) {
		provider, err := sandbox.NewProviderForPlatform("bubblewrap")

		if err != nil {
			t.Errorf("NewProviderForPlatform(bubblewrap) should succeed: %v", err)
		}

		if provider == nil {
			t.Error("Provider should not be nil for bubblewrap")
		}

		if provider != nil && provider.Name() != "bubblewrap" {
			t.Errorf("Provider name should be 'bubblewrap', got '%s'", provider.Name())
		}
	})

	t.Run("mock", func(t *testing.T) {
		provider, err := sandbox.NewProviderForPlatform("mock")

		if err != nil {
			t.Errorf("NewProviderForPlatform(mock) should succeed: %v", err)
		}

		if provider == nil {
			t.Error("Provider should not be nil for mock")
		}

		if provider != nil && provider.Name() != "mock" {
			t.Errorf("Provider name should be 'mock', got '%s'", provider.Name())
		}
	})
}

func TestNewProviderForPlatform_Unimplemented(t *testing.T) {
	// apfs is implemented on darwin; overlayfs is implemented on linux. The
	// rest remain unimplemented across all platforms.
	unimplementedProviders := []string{
		"fuse-overlayfs", // Not yet implemented
		"macfuse",        // macOS alternative
		"fallback",       // Generic fallback
	}

	for _, providerName := range unimplementedProviders {
		t.Run(providerName, func(t *testing.T) {
			provider, err := sandbox.NewProviderForPlatform(providerName)

			if err == nil {
				t.Errorf("NewProviderForPlatform(%s) should return error (not implemented)", providerName)
			}

			if provider != nil {
				t.Errorf("Provider should be nil for unimplemented provider %s", providerName)
			}

			// Verify it's a sandbox.Error with correct code
			var sbErr *sandbox.Error
			if err != nil && !errors.As(err, &sbErr) {
				t.Errorf("Error should be *sandbox.Error for %s", providerName)
			}

			if sbErr != nil && sbErr.Code != sandbox.ErrCodeUnsupportedPlatform {
				t.Errorf("Error code should be ErrCodeUnsupportedPlatform for %s, got %v", providerName, sbErr.Code)
			}
		})
	}
}

func TestNewProviderForPlatform_Unknown(t *testing.T) {
	provider, err := sandbox.NewProviderForPlatform("nonexistent-provider")

	if err == nil {
		t.Error("NewProviderForPlatform(unknown) should return error")
	}

	if provider != nil {
		t.Error("Provider should be nil for unknown provider")
	}

	var sbErr *sandbox.Error
	if err != nil && !errors.As(err, &sbErr) {
		t.Error("Error should be *sandbox.Error")
	}

	if sbErr != nil && sbErr.Code != sandbox.ErrCodeUnsupportedPlatform {
		t.Errorf("Error code should be ErrCodeUnsupportedPlatform, got %v", sbErr.Code)
	}
}

func TestNewProvider(t *testing.T) {
	// NewProvider should detect platform and return appropriate provider
	provider, err := sandbox.NewProvider()

	if runtime.GOOS == "linux" {
		// On Linux, should return bubblewrap or overlayfs provider depending on availability
		info, _ := sandbox.DetectPlatform()
		if info != nil {
			if err != nil {
				t.Errorf("NewProvider should succeed on Linux: %v", err)
			}
			if provider == nil {
				t.Error("Provider should not be nil")
			}
			// Accept bubblewrap or overlayfs-native depending on what's available
			if provider != nil {
				providerName := provider.Name()
				if providerName != "bubblewrap" && providerName != "overlayfs-native" {
					t.Errorf("Expected bubblewrap or overlayfs-native provider, got %s", providerName)
				}
				t.Logf("NewProvider returned: %s", providerName)
			}
		}
	} else {
		// On non-Linux platforms, providers may not be implemented yet
		t.Logf("NewProvider result on %s: err=%v, provider=%v", runtime.GOOS, err, provider)
	}
}
