//go:build darwin

// Package apfs provides a macOS APFS sandbox implementation using reflink cloning.
package apfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func init() {
	sandbox.RegisterProvider("apfs", func() sandbox.Provider {
		return NewProvider()
	})
}

// cloneTimeout bounds how long a single lower-dir clone may run. It exists
// so a misresolved (or malicious) lower dir — historically $HOME under a
// launchd context with no working directory set — cannot hang or run an
// unbounded copy: the clone is killed and cleaned up well inside this
// deadline instead of relying on an outer caller timeout (whose SIGKILL
// skips this package's own cleanup).
const cloneTimeout = 120 * time.Second

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
	workingDir := mergedDir
	if req.WorkingDir != "" {
		match, matchErr := sandbox.MatchWorkingDir(req.WorkingDir, req.LowerDirs)
		if matchErr != nil {
			return nil, matchErr
		}
		workingDir = filepath.Join(mergedDir, fmt.Sprintf("repo%d", match.LowerIndex), match.RelativeDir)
	}

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
		if err := p.cloneDirectory(ctx, lowerDir, cloneDir); err != nil {
			// Clean up on clone failure
			_ = os.RemoveAll(req.WorkspaceDir)
			return nil, sandbox.WrapError(sandbox.ErrCodeMountFailed,
				fmt.Sprintf("reflink clone failed for %s", lowerDir), err)
		}
		if err := p.detachLinkedWorktreeGitMetadata(ctx, lowerDir, cloneDir); err != nil {
			_ = os.RemoveAll(req.WorkspaceDir)
			return nil, sandbox.WrapError(sandbox.ErrCodeMountFailed,
				fmt.Sprintf("failed to isolate Git metadata for %s", lowerDir), err)
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
		WorkingDir: workingDir,
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
//
// The clone is bound to cloneTimeout and rejects a destination nested inside
// the source: without these guards, a misresolved src (e.g. $HOME) clones an
// unbounded tree into a dst underneath itself, growing without limit.
func (p *Provider) cloneDirectory(ctx context.Context, src, dst string) error {
	nested, err := isDstNestedInSrc(src, dst)
	if err != nil {
		return fmt.Errorf("failed to resolve clone paths: %w", err)
	}
	if nested {
		return fmt.Errorf("refusing to clone %s: destination %s is nested inside the source", src, dst)
	}

	cloneCtx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()

	// Try APFS reflink cloning first via cp -c
	// The -c flag uses clonefile() syscall on APFS for zero-copy CoW
	cmd := exec.CommandContext(cloneCtx, "cp", "-c", "-R", src, dst)
	// Process-group isolation + group-cancel + WaitDelay matches the
	// repo's subprocess-execution convention (see codexcontrol.Client):
	// SysProcAttr places cp in its own process group so a group-kill on
	// cancel takes any wedged descendant with it, and WaitDelay bounds how
	// long Wait() blocks on the command's I/O pipes after the context is
	// canceled — CommandContext's SIGKILL alone reaps only the direct
	// child; without WaitDelay a wedged descendant holding a pipe open can
	// still block Wait() indefinitely (see PR #915 / ce-fmxv for the same
	// failure mode in the codex boot path).
	configureIsolatedCommand(cmd)
	if runErr := cmd.Run(); runErr != nil {
		if cloneCtx.Err() != nil {
			_ = os.RemoveAll(dst)
			// Distinguish a genuine timeout from a parent-context cancellation
			// (e.g. caller interrupt): only DeadlineExceeded is a timeout.
			if cloneCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("clone of %s timed out after %s: %w", src, cloneTimeout, cloneCtx.Err())
			}
			return fmt.Errorf("clone of %s canceled: %w", src, cloneCtx.Err())
		}
		// Check if error is due to clonefile not supported (non-APFS filesystem)
		if isClonefileError(runErr) {
			// Warn that APFS is preferred but fall back
			fmt.Fprintf(os.Stderr, "Warning: APFS clonefile not supported, falling back to recursive copy\n")
			return p.copyDirectoryRecursive(cloneCtx, src, dst)
		}
		// Other errors (permissions, disk full, etc.) are real failures
		return fmt.Errorf("cp -c failed: %w", runErr)
	}
	return nil
}

// detachLinkedWorktreeGitMetadata replaces a cloned .git indirection with an
// independent copy-on-write Git directory. Linked worktree .git files contain
// an absolute path into the host repository; leaving that file in the clone
// would let sandbox Git commands mutate the host worktree index and refs.
func (p *Provider) detachLinkedWorktreeGitMetadata(ctx context.Context, src, dst string) error {
	detach, err := needsGitMetadataDetachment(src)
	if err != nil {
		return err
	}
	if !detach {
		return nil
	}

	detachCtx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	gitDir, commonDir, err := resolveLinkedGitDirectories(detachCtx, src)
	if err != nil {
		return err
	}
	stageParent, stageGit, err := p.stageDetachedGitMetadata(detachCtx, gitDir, commonDir, dst)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stageParent) }()
	return activateDetachedGitMetadata(detachCtx, stageGit, dst)
}

func needsGitMetadataDetachment(src string) (bool, error) {
	info, err := os.Lstat(filepath.Join(src, ".git"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect source Git metadata: %w", err)
	}
	if info.IsDir() {
		return false, nil
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("unsupported source .git entry mode %s", info.Mode())
	}
	return true, nil
}

func resolveLinkedGitDirectories(ctx context.Context, src string) (string, string, error) {
	gitDir, err := apfsGitOutput(ctx, "-C", src, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", "", fmt.Errorf("resolve linked worktree Git directory: %w", err)
	}
	commonDir, err := apfsGitOutput(ctx, "-C", src, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("resolve linked worktree common Git directory: %w", err)
	}
	return gitDir, commonDir, nil
}

func (p *Provider) stageDetachedGitMetadata(ctx context.Context, gitDir, commonDir, dst string) (string, string, error) {
	stageParent, err := os.MkdirTemp(filepath.Dir(dst), ".git-detach-")
	if err != nil {
		return "", "", fmt.Errorf("create Git metadata staging directory: %w", err)
	}
	stageGit := filepath.Join(stageParent, ".git")
	if err := p.cloneDirectory(ctx, commonDir, stageGit); err != nil {
		_ = os.RemoveAll(stageParent)
		return "", "", fmt.Errorf("clone common Git metadata: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(stageGit, "worktrees")); err != nil {
		_ = os.RemoveAll(stageParent)
		return "", "", fmt.Errorf("remove copied host worktree registrations: %w", err)
	}
	if err := copyLinkedWorktreeState(gitDir, stageGit); err != nil {
		_ = os.RemoveAll(stageParent)
		return "", "", err
	}
	return stageParent, stageGit, nil
}

func copyLinkedWorktreeState(gitDir, stageGit string) error {
	for _, name := range []string{"HEAD", "index", "config.worktree"} {
		destination := filepath.Join(stageGit, name)
		err := copyRegularFile(filepath.Join(gitDir, name), destination)
		if os.IsNotExist(err) && name != "HEAD" {
			if removeErr := os.RemoveAll(destination); removeErr != nil {
				return fmt.Errorf("remove inherited primary worktree %s: %w", name, removeErr)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("copy linked worktree %s: %w", name, err)
		}
	}
	return nil
}

func activateDetachedGitMetadata(ctx context.Context, stageGit, dst string) error {
	dstGit := filepath.Join(dst, ".git")
	if err := os.RemoveAll(dstGit); err != nil {
		return fmt.Errorf("remove cloned Git indirection: %w", err)
	}
	if err := os.Rename(stageGit, dstGit); err != nil {
		return fmt.Errorf("activate detached Git metadata: %w", err)
	}
	if err := configureDetachedGitWorktree(ctx, dstGit, dst); err != nil {
		return err
	}
	resolvedRoot, err := apfsGitOutput(ctx, "-C", dst, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("validate detached Git worktree: %w", err)
	}
	sameRoot, err := sameExistingPath(resolvedRoot, dst)
	if err != nil {
		return fmt.Errorf("compare detached Git worktree root: %w", err)
	}
	if !sameRoot {
		return fmt.Errorf("detached Git metadata resolved worktree %q, want %q", resolvedRoot, dst)
	}
	return nil
}

func configureDetachedGitWorktree(ctx context.Context, gitDir, worktree string) error {
	if _, err := apfsGitOutput(ctx, "config", "--file", filepath.Join(gitDir, "config"), "core.worktree", worktree); err != nil {
		return fmt.Errorf("configure detached Git worktree: %w", err)
	}
	worktreeConfig := filepath.Join(gitDir, "config.worktree")
	if _, err := os.Stat(worktreeConfig); err == nil {
		if _, err := apfsGitOutput(ctx, "config", "--file", worktreeConfig, "core.worktree", worktree); err != nil {
			return fmt.Errorf("configure detached per-worktree Git path: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect detached per-worktree Git config: %w", err)
	}
	return nil
}

func copyRegularFile(src, dst string) (returnErr error) {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); returnErr == nil && closeErr != nil {
			returnErr = closeErr
		}
	}()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// dst is inside the provider-owned staging directory.
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); returnErr == nil && closeErr != nil {
			returnErr = closeErr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}

func apfsGitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	configureIsolatedCommand(cmd)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func configureIsolatedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
}

func sameExistingPath(first, second string) (bool, error) {
	firstInfo, err := os.Stat(first)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", first, err)
	}
	secondInfo, err := os.Stat(second)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", second, err)
	}
	return os.SameFile(firstInfo, secondInfo), nil
}

// isDstNestedInSrc reports whether dst is inside (or equal to) src, which
// would make a directory clone from src to dst grow without bound. dst
// itself need not exist yet (the caller creates it via this clone).
func isDstNestedInSrc(src, dst string) (bool, error) {
	// Normalize to absolute paths first. filepath.EvalSymlinks preserves a
	// relative input as relative, and filepath.Rel cannot relate a relative
	// path to an absolute one (it errors), so a relative src or dst would make
	// the nesting check fail spuriously. filepath.Abs makes the comparison
	// well-defined regardless of the caller's cwd.
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return false, fmt.Errorf("failed to make source path %s absolute: %w", src, err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return false, fmt.Errorf("failed to make destination path %s absolute: %w", dst, err)
	}

	resolvedSrc, err := filepath.EvalSymlinks(absSrc)
	if err != nil {
		return false, fmt.Errorf("failed to resolve source %s: %w", src, err)
	}
	resolvedSrc = filepath.Clean(resolvedSrc)
	resolvedDst := resolvePathBestEffort(absDst)

	rel, err := filepath.Rel(resolvedSrc, resolvedDst)
	if err != nil {
		return false, fmt.Errorf("failed to compute relative path from %s to %s: %w", resolvedSrc, resolvedDst, err)
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

// resolvePathBestEffort resolves symlinks on the longest existing ancestor of
// path and rejoins any remaining (not-yet-created) components lexically.
// filepath.EvalSymlinks requires the full path to exist, which a clone
// destination does not until after the clone runs.
func resolvePathBestEffort(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	parent := filepath.Dir(path)
	if parent == path {
		return filepath.Clean(path)
	}
	return filepath.Join(resolvePathBestEffort(parent), filepath.Base(path))
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
func (p *Provider) copyDirectoryRecursive(ctx context.Context, src, dst string) error {
	// Walk source directory
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
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
