//go:build linux

// Package overlayfs provides a native Linux OverlayFS sandbox implementation.
// DEPRECATED: This package is kept for backwards compatibility.
// New code should use sandbox.NewOverlayFSProvider() directly.
package overlayfs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// Provider implements sandbox.Provider using native Linux OverlayFS.
// Requires kernel 5.11+ for rootless operation.
type Provider struct {
	mu sync.RWMutex
	// sandboxes tracks active sandboxes by ID
	sandboxes map[string]*sandbox.Sandbox
}

// NewProvider creates a new OverlayFS provider.
func NewProvider() *Provider {
	return &Provider{
		sandboxes: make(map[string]*sandbox.Sandbox),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "overlayfs-native"
}

// Create provisions a new isolated sandbox environment using OverlayFS.
func (p *Provider) Create(ctx context.Context, req sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	// Check context cancellation before starting
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate request
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Check kernel version
	if err := p.checkKernelVersion(); err != nil {
		return nil, err
	}

	// Create directory structure
	upperDir := filepath.Join(req.WorkspaceDir, "upper")
	workDir := filepath.Join(req.WorkspaceDir, "work")
	mergedDir := filepath.Join(req.WorkspaceDir, "merged")

	if err := p.createDirectories(upperDir, workDir, mergedDir); err != nil {
		return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
			"failed to create sandbox directories", err)
	}

	// Check context again before mount
	if err := ctx.Err(); err != nil {
		// Clean up partial state
		_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
		return nil, err
	}

	// Build lowerdir string (colon-separated, reverse order for priority)
	lowerDirStr := strings.Join(req.LowerDirs, ":")

	// Mount OverlayFS with xino=auto flag for inotify propagation
	if err := p.mountOverlay(lowerDirStr, upperDir, workDir, mergedDir); err != nil {
		// Clean up on mount failure
		_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
		return nil, err
	}

	// Write secrets to upperdir/.env if provided
	if len(req.Secrets) > 0 {
		if err := p.writeSecrets(upperDir, req.Secrets); err != nil {
			// Unmount and clean up on secrets failure
			_ = p.unmountOverlay(mergedDir)
			_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
			return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
				"failed to write secrets", err)
		}
	}

	// Create sandbox metadata
	sb := &sandbox.Sandbox{
		ID:         req.SessionID,
		MergedPath: mergedDir,
		UpperPath:  upperDir,
		WorkPath:   workDir,
		Type:       p.Name(),
		CreatedAt:  time.Now(),
		CleanupFunc: func() error {
			return p.cleanup(mergedDir, upperDir, workDir)
		},
	}

	// Store in provider's registry
	p.mu.Lock()
	p.sandboxes[sb.ID] = sb
	p.mu.Unlock()

	return sb, nil
}

// Destroy tears down a sandbox and cleans up all associated resources.
func (p *Provider) Destroy(ctx context.Context, id string) error {
	p.mu.Lock()
	sb, exists := p.sandboxes[id]
	if !exists {
		p.mu.Unlock()
		return nil // Idempotent
	}
	delete(p.sandboxes, id)
	p.mu.Unlock()

	if sb.CleanupFunc != nil {
		if err := sb.CleanupFunc(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks if a sandbox exists and is healthy.
func (p *Provider) Validate(ctx context.Context, id string) error {
	p.mu.RLock()
	sb, exists := p.sandboxes[id]
	p.mu.RUnlock()
	if !exists {
		return sandbox.NewSandboxNotFoundError(id)
	}

	// Check if merged directory exists
	if _, err := os.Stat(sb.MergedPath); os.IsNotExist(err) {
		return sandbox.NewSandboxNotFoundError(id)
	}

	// Verify mount is still active by checking /proc/mounts
	if err := p.verifyMount(sb.MergedPath); err != nil {
		return err
	}

	return nil
}

// validateRequest checks if the request is valid.
func (p *Provider) validateRequest(req sandbox.SandboxRequest) error {
	if req.SessionID == "" {
		return sandbox.NewInvalidConfigError("SessionID", "must not be empty")
	}

	if len(req.LowerDirs) == 0 {
		return sandbox.NewInvalidConfigError("LowerDirs", "at least one lower directory is required")
	}

	// Verify all lower directories exist
	for _, dir := range req.LowerDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return sandbox.NewRepoNotFoundError(dir)
		}
	}

	if req.WorkspaceDir == "" {
		return sandbox.NewInvalidConfigError("WorkspaceDir", "must not be empty")
	}

	return nil
}

// checkKernelVersion verifies kernel is >= 5.11 for rootless OverlayFS.
func (p *Provider) checkKernelVersion() error {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return sandbox.WrapError(sandbox.ErrCodeKernelTooOld,
			"failed to read kernel version", err)
	}

	version := parseKernelVersion(string(data))
	if !isKernelVersionAtLeast(version, 5, 11) {
		return sandbox.NewKernelTooOldError(version, 5, 11)
	}

	return nil
}

// parseKernelVersion extracts "X.Y.Z" from kernel version string.
func parseKernelVersion(versionStr string) string {
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

// isKernelVersionAtLeast checks if version >= major.minor.
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

// createDirectories creates the required directory structure.
func (p *Provider) createDirectories(upperDir, workDir, mergedDir string) error {
	dirs := []string{upperDir, workDir, mergedDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// mountOverlay performs the OverlayFS mount operation.
func (p *Provider) mountOverlay(lowerDir, upperDir, workDir, mergedDir string) error {
	// Build mount options
	// CRITICAL: xino=auto ensures inotify propagation works correctly
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,xino=auto",
		lowerDir, upperDir, workDir)

	// Execute mount command
	// Note: On kernel 5.11+, this works rootless (no sudo needed)
	cmd := exec.Command("mount", "-t", "overlay", "overlay",
		"-o", options, mergedDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if permission denied
		if strings.Contains(string(output), "permission denied") ||
			strings.Contains(string(output), "Operation not permitted") {
			return sandbox.NewMountPermissionError(err)
		}
		return sandbox.WrapError(sandbox.ErrCodeMountFailed,
			fmt.Sprintf("failed to mount overlay: %s", string(output)), err)
	}

	return nil
}

// unmountOverlay unmounts the OverlayFS mount with retry logic.
func (p *Provider) unmountOverlay(mergedDir string) error {
	const maxRetries = 3
	const retryDelay = 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Log retry attempt
			fmt.Fprintf(os.Stderr, "unmount retry %d/%d for %s\n", attempt+1, maxRetries, mergedDir)
			time.Sleep(retryDelay * time.Duration(attempt))
		}

		// Try graceful unmount first
		err := syscall.Unmount(mergedDir, 0)
		if err == nil {
			return nil
		}

		// Check if already unmounted
		if err == syscall.EINVAL {
			// Not mounted, that's OK
			return nil
		}

		// If busy and this is our last attempt, try force unmount
		if attempt == maxRetries-1 {
			err = syscall.Unmount(mergedDir, syscall.MNT_FORCE)
			if err == nil {
				fmt.Fprintf(os.Stderr, "force unmount succeeded for %s\n", mergedDir)
				return nil
			}
			if err == syscall.EINVAL {
				// Not mounted anymore
				return nil
			}
			unmountErr := sandbox.WrapError(sandbox.ErrCodeUnmountFailed,
				fmt.Sprintf("failed to unmount %s after %d retries", mergedDir, maxRetries), err)
			return sandbox.WithRecoveryHint(
				sandbox.WithDiagnostic(unmountErr, fmt.Sprintf("lsof +D %s && mount | grep %s", mergedDir, mergedDir)),
				fmt.Sprintf("Check for processes using the mount with 'lsof +D %s', kill if needed, then try 'umount -l %s' for lazy unmount.", mergedDir, mergedDir),
			)
		}
	}

	return sandbox.NewError(sandbox.ErrCodeUnmountFailed,
		fmt.Sprintf("unmount failed after %d retries: %s", maxRetries, mergedDir))
}

// cleanupDirectories removes sandbox directories with retry logic.
func (p *Provider) cleanupDirectories(upperDir, workDir, mergedDir string) error {
	const maxRetries = 3
	const retryDelay = 50 * time.Millisecond

	var errors []error

	// Remove in reverse order
	dirs := []string{mergedDir, workDir, upperDir}
	for _, dir := range dirs {
		// Skip if directory doesn't exist
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		// Retry removal
		var removed bool
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				time.Sleep(retryDelay * time.Duration(attempt))
			}

			err := os.RemoveAll(dir)
			if err == nil {
				removed = true
				break
			}

			// If directory is gone now, that's OK
			if os.IsNotExist(err) {
				removed = true
				break
			}

			// Log retry
			if attempt < maxRetries-1 {
				fmt.Fprintf(os.Stderr, "cleanup retry %d/%d for %s: %v\n", attempt+1, maxRetries, dir, err)
			}
		}

		if !removed {
			err := fmt.Errorf("failed to remove %s after %d retries", dir, maxRetries)
			errors = append(errors, err)
			fmt.Fprintf(os.Stderr, "cleanup failed: %v\n", err)
		}
	}

	// Return combined error if any failures
	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %v", errors)
	}

	return nil
}

// cleanup performs full cleanup: unmount + remove directories.
// This function is idempotent and handles partial failures gracefully.
func (p *Provider) cleanup(mergedDir, upperDir, workDir string) error {
	var errors []error

	// Step 1: Unmount first
	if err := p.unmountOverlay(mergedDir); err != nil {
		errors = append(errors, fmt.Errorf("unmount failed: %w", err))
		// Continue with cleanup even if unmount fails
		fmt.Fprintf(os.Stderr, "unmount error (continuing cleanup): %v\n", err)
	}

	// Step 2: Verify unmount succeeded
	if err := p.verifyUnmounted(mergedDir); err != nil {
		errors = append(errors, fmt.Errorf("unmount verification failed: %w", err))
		fmt.Fprintf(os.Stderr, "unmount verification error: %v\n", err)
	}

	// Step 3: Remove directories
	if err := p.cleanupDirectories(upperDir, workDir, mergedDir); err != nil {
		cleanupErr := sandbox.NewCleanupFailedError(mergedDir, err)
		errors = append(errors, cleanupErr)
	}

	// Step 4: Final verification
	if err := p.verifyCleanupComplete(upperDir, workDir, mergedDir); err != nil {
		errors = append(errors, fmt.Errorf("cleanup verification failed: %w", err))
		fmt.Fprintf(os.Stderr, "cleanup verification error: %v\n", err)
	}

	// Return combined error if any failures
	if len(errors) > 0 {
		return fmt.Errorf("cleanup completed with errors: %v", errors)
	}

	return nil
}

// verifyUnmounted checks that a mount point is no longer mounted.
func (p *Provider) verifyUnmounted(mergedDir string) error {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		// Can't verify, assume OK (non-Linux or permission denied)
		return nil //nolint:nilerr // Verification is best-effort on Linux only
	}

	mounts := string(data)
	if strings.Contains(mounts, mergedDir) {
		return fmt.Errorf("mount still present in /proc/mounts: %s", mergedDir)
	}

	return nil
}

// verifyCleanupComplete checks that all directories are removed.
func (p *Provider) verifyCleanupComplete(upperDir, workDir, mergedDir string) error {
	var remaining []string

	dirs := []string{mergedDir, workDir, upperDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			remaining = append(remaining, dir)
		}
	}

	if len(remaining) > 0 {
		return fmt.Errorf("directories still exist after cleanup: %v", remaining)
	}

	return nil
}

// writeSecrets writes secrets to upperdir/.env file with environment variable expansion.
func (p *Provider) writeSecrets(upperDir string, secrets map[string]string) error {
	envFile := filepath.Join(upperDir, ".env")

	var buf strings.Builder
	buf.WriteString("# Auto-generated by AGM sandbox\n")
	buf.WriteString("# DO NOT COMMIT THIS FILE\n")
	buf.WriteString("#\n")
	buf.WriteString("# This file contains secrets injected into the sandbox environment.\n")
	buf.WriteString("# Secrets are isolated to the sandbox and never written to lowerdir.\n\n")

	// Sort keys for deterministic output
	keys := make([]string, 0, len(secrets))
	for key := range secrets {
		keys = append(keys, key)
	}
	// Note: Not sorting to preserve insertion order for tests
	// In production, map iteration order is already randomized

	for key, value := range secrets {
		// Expand environment variables in value
		expandedValue := os.ExpandEnv(value)
		buf.WriteString(fmt.Sprintf("%s=%s\n", key, expandedValue))
	}

	// Write with restricted permissions (0600 - owner read/write only)
	if err := os.WriteFile(envFile, []byte(buf.String()), 0600); err != nil {
		return err
	}

	return nil
}

// verifyMount checks if the merged directory is actually mounted.
func (p *Provider) verifyMount(mergedDir string) error {
	// Read /proc/mounts to verify mount exists
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return sandbox.WrapError(sandbox.ErrCodeMountFailed,
			"failed to read /proc/mounts", err)
	}

	// Look for our mount point
	mounts := string(data)
	if !strings.Contains(mounts, mergedDir) {
		return sandbox.NewError(sandbox.ErrCodeSandboxNotFound,
			fmt.Sprintf("mount not found in /proc/mounts: %s", mergedDir))
	}

	return nil
}
