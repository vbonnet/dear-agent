//go:build linux

// Package sandbox provides OverlayFS-based sandboxing for Linux systems.
package sandbox

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
)

func init() {
	// Register the overlayfs provider
	RegisterProvider("overlayfs", func() Provider {
		return NewOverlayFSProvider()
	})
}

// OverlayFSProvider implements Provider using native Linux OverlayFS.
// Requires kernel 5.11+ for rootless operation.
type OverlayFSProvider struct {
	mu sync.RWMutex
	// sandboxes tracks active sandboxes by ID
	sandboxes map[string]*Sandbox
}

// NewOverlayFSProvider creates a new OverlayFS provider.
func NewOverlayFSProvider() *OverlayFSProvider {
	return &OverlayFSProvider{
		sandboxes: make(map[string]*Sandbox),
	}
}

// Name returns the provider name.
func (p *OverlayFSProvider) Name() string {
	return "overlayfs-native"
}

// Create provisions a new isolated sandbox environment using OverlayFS.
func (p *OverlayFSProvider) Create(ctx context.Context, req SandboxRequest) (*Sandbox, error) {
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
		return nil, WrapError(ErrCodePermissionDenied,
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
			return nil, WrapError(ErrCodePermissionDenied,
				"failed to write secrets", err)
		}
	}

	// Create sandbox metadata
	sb := &Sandbox{
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
func (p *OverlayFSProvider) Destroy(ctx context.Context, id string) error {
	p.mu.Lock()
	sb, exists := p.sandboxes[id]
	if !exists {
		p.mu.Unlock()
		return nil
	}
	delete(p.sandboxes, id)
	p.mu.Unlock()

	// Call cleanup function if present (outside lock to avoid holding during I/O)
	if sb.CleanupFunc != nil {
		if err := sb.CleanupFunc(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks if a sandbox exists and is healthy.
func (p *OverlayFSProvider) Validate(ctx context.Context, id string) error {
	p.mu.RLock()
	sb, exists := p.sandboxes[id]
	p.mu.RUnlock()
	if !exists {
		return NewError(ErrCodeSandboxNotFound,
			fmt.Sprintf("sandbox not found: %s", id))
	}

	// Check if merged directory exists
	if _, err := os.Stat(sb.MergedPath); os.IsNotExist(err) {
		return NewError(ErrCodeSandboxNotFound,
			fmt.Sprintf("sandbox merged directory not found: %s", sb.MergedPath))
	}

	// Verify mount is still active by checking /proc/mounts
	if err := p.verifyMount(sb.MergedPath); err != nil {
		return err
	}

	return nil
}

// validateRequest checks if the request is valid.
func (p *OverlayFSProvider) validateRequest(req SandboxRequest) error {
	if req.SessionID == "" {
		return NewError(ErrCodeInvalidConfig, "session ID is required")
	}

	if len(req.LowerDirs) == 0 {
		return NewError(ErrCodeInvalidConfig, "at least one lower directory is required")
	}

	// Verify all lower directories exist
	for _, dir := range req.LowerDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return WrapError(ErrCodeRepoNotFound,
				fmt.Sprintf("lower directory not found: %s", dir), err)
		}
	}

	if req.WorkspaceDir == "" {
		return NewError(ErrCodeInvalidConfig, "workspace directory is required")
	}

	return nil
}

// checkKernelVersion verifies kernel is >= 5.11 for rootless OverlayFS.
func (p *OverlayFSProvider) checkKernelVersion() error {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return WrapError(ErrCodeKernelTooOld,
			"failed to read kernel version", err)
	}

	version := parseKernelVersion(string(data))
	if !isKernelVersionAtLeast(version, 5, 11) {
		return NewError(ErrCodeKernelTooOld,
			fmt.Sprintf("kernel version %s is too old, need 5.11+", version))
	}

	return nil
}

// createDirectories creates the required directory structure.
func (p *OverlayFSProvider) createDirectories(upperDir, workDir, mergedDir string) error {
	dirs := []string{upperDir, workDir, mergedDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// mountOverlay performs the OverlayFS mount operation.
func (p *OverlayFSProvider) mountOverlay(lowerDir, upperDir, workDir, mergedDir string) error {
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
		return WrapError(ErrCodeMountFailed,
			fmt.Sprintf("failed to mount overlay: %s", string(output)), err)
	}

	return nil
}

// unmountOverlay unmounts the OverlayFS mount.
func (p *OverlayFSProvider) unmountOverlay(mergedDir string) error {
	// Try graceful unmount first
	err := syscall.Unmount(mergedDir, 0)
	if err == nil {
		return nil
	}

	// If busy, try force unmount
	err = syscall.Unmount(mergedDir, syscall.MNT_FORCE)
	if err != nil {
		// Check if already unmounted
		if err == syscall.EINVAL {
			// Not mounted, that's OK
			return nil
		}
		return WrapError(ErrCodeUnmountFailed,
			fmt.Sprintf("failed to unmount %s", mergedDir), err)
	}

	return nil
}

// cleanupDirectories removes sandbox directories.
func (p *OverlayFSProvider) cleanupDirectories(upperDir, workDir, mergedDir string) error {
	var lastErr error

	// Remove in reverse order
	dirs := []string{mergedDir, workDir, upperDir}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// cleanup performs full cleanup: unmount + remove directories.
func (p *OverlayFSProvider) cleanup(mergedDir, upperDir, workDir string) error {
	// Unmount first
	if err := p.unmountOverlay(mergedDir); err != nil {
		return err
	}

	// Then remove directories
	return p.cleanupDirectories(upperDir, workDir, mergedDir)
}

// writeSecrets writes secrets to upperdir/.env file.
func (p *OverlayFSProvider) writeSecrets(upperDir string, secrets map[string]string) error {
	envFile := filepath.Join(upperDir, ".env")

	var lines []string
	for key, value := range secrets {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	content := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		return err
	}

	return nil
}

// verifyMount checks if the merged directory is actually mounted.
func (p *OverlayFSProvider) verifyMount(mergedDir string) error {
	// Read /proc/mounts to verify mount exists
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return WrapError(ErrCodeMountFailed,
			"failed to read /proc/mounts", err)
	}

	// Look for our mount point
	mounts := string(data)
	if !strings.Contains(mounts, mergedDir) {
		return NewError(ErrCodeSandboxNotFound,
			fmt.Sprintf("mount not found in /proc/mounts: %s", mergedDir))
	}

	return nil
}
