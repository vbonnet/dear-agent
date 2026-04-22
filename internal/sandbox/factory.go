// Package sandbox provides sandbox functionality.
package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// providerRegistry holds registered provider factory functions
var (
	providerRegistry = make(map[string]func() Provider)
	registryMu       sync.RWMutex
)

// RegisterProvider registers a provider factory function.
// This should be called from provider package init() functions.
func RegisterProvider(name string, factory func() Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	providerRegistry[name] = factory
}

// PlatformInfo describes the detected runtime environment.
type PlatformInfo struct {
	OS            string // "linux", "darwin", "windows"
	KernelVersion string // "6.6.123" (Linux only)
	HasOverlayFS  bool   // Native OverlayFS support
	HasAPFS       bool   // APFS support (macOS)
	Recommended   string // "overlayfs", "apfs", "fallback"
}

// DetectPlatform analyzes the runtime environment.
func DetectPlatform() (*PlatformInfo, error) {
	info := &PlatformInfo{
		OS: runtime.GOOS,
	}

	switch runtime.GOOS {
	case "linux":
		return detectLinux(info)
	case "darwin":
		return detectMacOS(info)
	default:
		info.Recommended = "fallback"
		return info, nil
	}
}

func detectLinux(info *PlatformInfo) (*PlatformInfo, error) {
	// Read kernel version from /proc/version
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/version: %w", err)
	}

	version := parseKernelVersion(string(data))
	info.KernelVersion = version

	// Check if kernel supports native rootless OverlayFS (5.11+)
	info.HasOverlayFS = isKernelVersionAtLeast(version, 5, 11)

	// Recommend provider based on available capabilities
	// Priority: bubblewrap (works everywhere) > overlayfs (requires caps) > fuse-overlayfs
	switch {
	case hasBubblewrap():
		info.Recommended = "bubblewrap"
	case info.HasOverlayFS:
		info.Recommended = "overlayfs"
	default:
		info.Recommended = "fuse-overlayfs"
	}

	return info, nil
}

// hasBubblewrap checks if bubblewrap (bwrap) is available.
func hasBubblewrap() bool {
	// Check PATH first (most reliable)
	if _, err := exec.LookPath("bwrap"); err == nil {
		return true
	}
	// Check well-known locations
	paths := []string{
		"/usr/bin/bwrap",
		"/home/linuxbrew/.linuxbrew/bin/bwrap",
	}
	// Check user Homebrew location
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.linuxbrew/bin/bwrap")
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func detectMacOS(info *PlatformInfo) (*PlatformInfo, error) {
	// Check APFS support (macOS 10.13+)
	// For now, assume all modern macOS have APFS
	info.HasAPFS = true
	info.Recommended = "apfs"
	return info, nil
}

// parseKernelVersion extracts "X.Y.Z" from kernel version string.
// Example: "Linux version 6.6.123+ ..." → "6.6.123"
func parseKernelVersion(versionStr string) string {
	// Simple extraction: find "version X.Y.Z"
	parts := strings.Fields(versionStr)
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			version := parts[i+1]
			// Remove any trailing "+" or "-"
			version = strings.TrimRight(version, "+-")
			return version
		}
	}
	return "unknown"
}

// isKernelVersionAtLeast checks if version >= major.minor
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

// NewProvider creates the appropriate provider for the current platform.
func NewProvider() (Provider, error) {
	info, err := DetectPlatform()
	if err != nil {
		return nil, fmt.Errorf("platform detection failed: %w", err)
	}

	return NewProviderForPlatform(info.Recommended)
}

// NewProviderForPlatform creates a specific provider by name.
func NewProviderForPlatform(name string) (Provider, error) {
	// Check built-in providers first
	if name == "mock" {
		return NewMockProvider(), nil
	}

	// Check registered providers
	registryMu.RLock()
	factory, exists := providerRegistry[name]
	registryMu.RUnlock()

	if exists {
		return factory(), nil
	}

	// Return error for unimplemented/unknown providers
	return nil, NewError(ErrCodeUnsupportedPlatform, "provider not available: "+name)
}
