//go:build darwin

// Package apfs provides a macOS APFS sandbox implementation using reflink cloning.
package apfs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func init() {
	sandbox.RegisterProvider("apfs", func() sandbox.Provider {
		return NewProvider()
	})
}

// Provider implements sandbox.Provider using APFS reflink cloning.
// On macOS, there is no native union mount like OverlayFS, so we:
// 1. Clone each lowerdir using APFS reflinks (zero-copy, CoW)
// 2. Create a merged directory as a symlink to the cloned directory
// 3. Agent operates in the cloned dir (modifications use CoW)
type Provider struct {
	mu sync.RWMutex
	// sandboxes tracks active sandboxes by ID
	sandboxes map[string]*sandbox.Sandbox
}

// NewProvider creates a new APFS provider.
func NewProvider() *Provider {
	return &Provider{
		sandboxes: make(map[string]*sandbox.Sandbox),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "apfs-reflink"
}

// Create provisions a new isolated sandbox environment using APFS reflinks.
func (p *Provider) Create(ctx context.Context, req sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	// Check context cancellation before starting
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate request
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Create directory structure
	upperDir := filepath.Join(req.WorkspaceDir, "upper")
	mergedDir := filepath.Join(req.WorkspaceDir, "merged")

	if err := os.MkdirAll(upperDir, 0755); err != nil {
		return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
			"failed to create upperdir", err)
	}

	// Check context again before cloning
	if err := ctx.Err(); err != nil {
		// Clean up partial state
		_ = os.RemoveAll(req.WorkspaceDir)
		return nil, err
	}

	// Clone each lowerdir using APFS reflinks
	// For simplicity in MVP, we clone all into a single merged structure
	// Real implementation would merge them properly
	for i, lowerDir := range req.LowerDirs {
		cloneDir := filepath.Join(upperDir, fmt.Sprintf("repo%d", i))
		if err := p.cloneDirectory(lowerDir, cloneDir); err != nil {
			// Clean up on clone failure
			_ = os.RemoveAll(req.WorkspaceDir)
			return nil, sandbox.WrapError(sandbox.ErrCodeMountFailed,
				fmt.Sprintf("reflink clone failed for %s", lowerDir), err)
		}
	}

	// Create merged as symlink to upperdir
	// On macOS, without union mount, merged == cloned repos
	// Agent will operate directly in the cloned directories
	if err := os.Symlink(upperDir, mergedDir); err != nil {
		// Clean up on symlink failure
		_ = os.RemoveAll(req.WorkspaceDir)
		return nil, sandbox.WrapError(sandbox.ErrCodeMountFailed,
			"failed to create merged symlink", err)
	}

	// Write secrets to upperdir/.env if provided
	if len(req.Secrets) > 0 {
		if err := p.writeSecrets(upperDir, req.Secrets); err != nil {
			// Clean up on secrets failure
			_ = os.RemoveAll(req.WorkspaceDir)
			return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
				"failed to write secrets", err)
		}
	}

	// Create sandbox metadata
	sb := &sandbox.Sandbox{
		ID:         req.SessionID,
		MergedPath: mergedDir,
		UpperPath:  upperDir,
		WorkPath:   "", // Not used on macOS (no overlay workdir)
		Type:       p.Name(),
		CreatedAt:  time.Now(),
		CleanupFunc: func() error {
			return p.cleanup(req.WorkspaceDir)
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

	// Check if it's a valid symlink
	target, err := os.Readlink(sb.MergedPath)
	if err != nil {
		return sandbox.WrapError(sandbox.ErrCodeSandboxNotFound,
			fmt.Sprintf("merged path is not a valid symlink: %s", sb.MergedPath), err)
	}

	// Verify target exists
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return sandbox.NewSandboxNotFoundError(id)
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

// cloneDirectory uses APFS clonefile for zero-copy cloning.
// On APFS volumes, this uses "cp -c" which invokes clonefile syscall for CoW semantics.
// Falls back to recursive copy on non-APFS filesystems or when clonefile fails.
func (p *Provider) cloneDirectory(src, dst string) error {
	// Try APFS reflink cloning first via cp -c
	// The -c flag uses clonefile() syscall on APFS for zero-copy CoW
	cmd := exec.Command("cp", "-c", "-R", src, dst)
	if err := cmd.Run(); err != nil {
		// Check if error is due to clonefile not supported (non-APFS filesystem)
		if isClonefileError(err) {
			// Warn that APFS is preferred but fall back
			fmt.Fprintf(os.Stderr, "Warning: APFS clonefile not supported, falling back to recursive copy\n")
			return p.copyDirectoryRecursive(src, dst)
		}
		// Other errors (permissions, disk full, etc.) are real failures
		return fmt.Errorf("cp -c failed: %w", err)
	}
	return nil
}

// isClonefileError detects if cp -c failed due to clonefile not being supported.
// This happens on non-APFS filesystems (HFS+, NFS, SMB, etc.)
func isClonefileError(err error) bool {
	if err == nil {
		return false
	}
	// cp -c returns exit code 1 with specific error when clonefile unsupported
	// Error message contains "cloning not supported" or similar
	errStr := err.Error()
	return strings.Contains(errStr, "cloning not supported") ||
		strings.Contains(errStr, "Operation not supported") ||
		strings.Contains(errStr, "not supported")
}

// copyDirectoryRecursive performs a recursive directory copy.
// This is a fallback implementation. Real APFS provider should use clonefile.
func (p *Provider) copyDirectoryRecursive(src, dst string) error {
	// Walk source directory
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dstPath) //nolint:gosec // G122: trusted local paths, symlink TOCTOU not in threat model
		}

		// Handle regular files
		data, err := os.ReadFile(path) //nolint:gosec // G122: trusted local paths, symlink TOCTOU not in threat model
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// cleanup removes the workspace directory with retry logic.
// This function is idempotent and handles partial failures gracefully.
func (p *Provider) cleanup(workspaceDir string) error {
	const maxRetries = 3
	const retryDelay = 50 * time.Millisecond

	// Skip if directory doesn't exist
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		return nil
	}

	// Retry removal with exponential backoff
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Log retry attempt
			fmt.Fprintf(os.Stderr, "cleanup retry %d/%d for %s\n", attempt+1, maxRetries, workspaceDir)
			time.Sleep(retryDelay * time.Duration(attempt))
		}

		err := os.RemoveAll(workspaceDir)
		if err == nil {
			// Verify cleanup succeeded
			if _, statErr := os.Stat(workspaceDir); os.IsNotExist(statErr) {
				return nil
			}
			// Directory still exists, retry
			lastErr = fmt.Errorf("directory still exists after RemoveAll")
			continue
		}

		// If directory is gone now, that's OK
		if os.IsNotExist(err) {
			return nil
		}

		lastErr = err

		// On macOS, handle specific errors
		if isLockedFileError(err) {
			fmt.Fprintf(os.Stderr, "locked file detected, will retry: %v\n", err)
			continue
		}
	}

	// Final verification
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		// Directory is gone despite errors, cleanup succeeded
		return nil
	}

	return sandbox.NewCleanupFailedError(workspaceDir, lastErr)
}

// isLockedFileError checks if the error is due to locked files on macOS.
func isLockedFileError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "operation not permitted") ||
		strings.Contains(errStr, "resource busy") ||
		strings.Contains(errStr, "device busy")
}

// writeSecrets writes secrets to upperdir/.env file.
func (p *Provider) writeSecrets(upperDir string, secrets map[string]string) error {
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
