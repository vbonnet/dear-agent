//go:build linux

// Package gvisor provides a Linux sandbox implementation backed by gVisor
// (the `runsc` runtime). gVisor intercepts application syscalls in userspace
// via the ptrace or KVM platform, providing a stronger isolation boundary
// than namespace-based sandboxes such as bubblewrap. It is Linux-only because
// the underlying mechanisms (ptrace, KVM, Linux namespaces) do not exist on
// macOS or Windows.
//
// This provider mirrors the bubblewrap provider's filesystem layout: it
// creates upper/, work/, and merged/ directories under WorkspaceDir, and
// populates merged/ from the first git repository found in LowerDirs by
// adding a worktree on a per-session branch. Subsequent sandboxed execution
// (handled by callers, not this provider) wraps commands with `runsc run`
// against an OCI bundle that uses merged/ as its rootfs.
package gvisor

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

// Provider implements sandbox.Provider using gVisor's runsc runtime.
type Provider struct {
	mu        sync.RWMutex
	sandboxes map[string]*sandbox.Sandbox
}

// NewProvider creates a new gVisor provider.
func NewProvider() *Provider {
	return &Provider{
		sandboxes: make(map[string]*sandbox.Sandbox),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gvisor"
}

// Create provisions a new isolated sandbox using gVisor.
//
// The merged path is materialized as a git worktree of the first git repo in
// LowerDirs (matching bubblewrap), giving callers a writable working tree on
// an isolated branch with a proper .git directory. If no git repo is found,
// falls back to a symlink-populated merged dir.
func (p *Provider) Create(ctx context.Context, req sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	if err := p.checkRunscInstalled(); err != nil {
		return nil, err
	}

	upperDir := filepath.Join(req.WorkspaceDir, "upper")
	workDir := filepath.Join(req.WorkspaceDir, "work")
	mergedDir := filepath.Join(req.WorkspaceDir, "merged")

	if err := p.createDirectories(upperDir, workDir, mergedDir); err != nil {
		return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
			"failed to create sandbox directories", err)
	}

	worktreeRepo, worktreeCreated := p.tryCreateWorktree(req.LowerDirs, req.SessionID, mergedDir, req.TargetRepo)
	if !worktreeCreated {
		fmt.Fprintf(os.Stderr, "gvisor: no git repo in lower dirs, falling back to symlinks\n")
		if err := p.populateMergedDir(req.LowerDirs, mergedDir); err != nil {
			_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
			return nil, sandbox.WrapError(sandbox.ErrCodeMountFailed,
				"failed to populate merged directory with repo symlinks", err)
		}
	}

	if err := p.testRunsc(ctx); err != nil {
		if worktreeCreated {
			_ = p.removeWorktree(worktreeRepo, mergedDir)
		}
		_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
		return nil, err
	}

	if len(req.Secrets) > 0 {
		if err := p.writeSecrets(upperDir, req.Secrets); err != nil {
			if worktreeCreated {
				_ = p.removeWorktree(worktreeRepo, mergedDir)
			}
			_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
			return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
				"failed to write secrets", err)
		}
	}

	cleanupFn := func() error {
		if worktreeCreated {
			if err := p.removeWorktree(worktreeRepo, mergedDir); err != nil {
				fmt.Fprintf(os.Stderr, "gvisor: warning: failed to remove worktree: %v\n", err)
			}
		}
		return p.cleanup(upperDir, workDir, mergedDir)
	}

	sb := &sandbox.Sandbox{
		ID:          req.SessionID,
		MergedPath:  mergedDir,
		UpperPath:   upperDir,
		WorkPath:    workDir,
		Type:        p.Name(),
		CreatedAt:   time.Now(),
		CleanupFunc: cleanupFn,
	}

	p.mu.Lock()
	p.sandboxes[sb.ID] = sb
	p.mu.Unlock()
	return sb, nil
}

// Destroy tears down the sandbox and cleans up resources.
func (p *Provider) Destroy(ctx context.Context, id string) error {
	p.mu.Lock()
	sb, exists := p.sandboxes[id]
	if !exists {
		p.mu.Unlock()
		return nil
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

// Validate checks if a sandbox exists and its merged path is still present.
func (p *Provider) Validate(ctx context.Context, id string) error {
	p.mu.RLock()
	sb, exists := p.sandboxes[id]
	p.mu.RUnlock()
	if !exists {
		return sandbox.NewSandboxNotFoundError(id)
	}
	if _, err := os.Stat(sb.MergedPath); os.IsNotExist(err) {
		return sandbox.NewSandboxNotFoundError(id)
	}
	return nil
}

func (p *Provider) validateRequest(req sandbox.SandboxRequest) error {
	if req.SessionID == "" {
		return sandbox.NewInvalidConfigError("SessionID", "must not be empty")
	}
	if len(req.LowerDirs) == 0 {
		return sandbox.NewInvalidConfigError("LowerDirs", "at least one lower directory is required")
	}
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

// checkRunscInstalled verifies the runsc binary is available in PATH.
func (p *Provider) checkRunscInstalled() error {
	if _, err := exec.LookPath("runsc"); err != nil {
		return sandbox.NewError(sandbox.ErrCodeUnsupportedPlatform,
			"gvisor (runsc) not found in PATH - install from https://gvisor.dev/docs/user_guide/install/")
	}
	return nil
}

// testRunsc runs `runsc --version` to verify the binary is functional. We
// deliberately do not exercise `runsc run` here because that requires either
// CAP_SYS_ADMIN or a configured KVM platform, neither of which is universally
// available in the development and CI environments where Create() is called.
// Failures of the actual sandboxed execution surface to the caller that wraps
// commands with runsc, not to this provider.
func (p *Provider) testRunsc(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "runsc", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return sandbox.WrapError(sandbox.ErrCodeMountFailed,
			fmt.Sprintf("runsc --version failed: %s", string(output)), err)
	}
	if !strings.Contains(strings.ToLower(string(output)), "runsc") {
		return sandbox.NewError(sandbox.ErrCodeMountFailed,
			"runsc --version did not produce expected output")
	}
	return nil
}

// tryCreateWorktree attempts to create a git worktree in mergedDir from the
// first git repo found in lowerDirs. Returns the repo path and true on
// success. If targetRepo is set, it is used directly instead of scanning.
func (p *Provider) tryCreateWorktree(lowerDirs []string, sessionID, mergedDir, targetRepo string) (string, bool) {
	var repoPath string
	if targetRepo != "" && p.isGitRepo(targetRepo) {
		repoPath = targetRepo
	} else {
		repoPath = p.findGitRepo(lowerDirs)
	}
	if repoPath == "" {
		return "", false
	}

	if err := os.RemoveAll(mergedDir); err != nil {
		fmt.Fprintf(os.Stderr, "gvisor: failed to remove mergedDir for worktree: %v\n", err)
		return "", false
	}

	branchName := "agm/" + sessionID
	if err := p.addWorktree(repoPath, mergedDir, branchName); err != nil {
		fmt.Fprintf(os.Stderr, "gvisor: git worktree add failed: %v\n", err)
		_ = os.MkdirAll(mergedDir, 0755)
		return "", false
	}
	return repoPath, true
}

// findGitRepo finds the first git repository among the lower directories.
func (p *Provider) findGitRepo(lowerDirs []string) string {
	for _, dir := range lowerDirs {
		resolved := dir
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			resolved = r
		}
		if p.isGitRepo(resolved) {
			return resolved
		}
		entries, err := os.ReadDir(resolved)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			subPath := filepath.Join(resolved, e.Name())
			info, err := os.Stat(subPath)
			if err != nil || !info.IsDir() {
				continue
			}
			if r, err := filepath.EvalSymlinks(subPath); err == nil {
				subPath = r
			}
			if p.isGitRepo(subPath) {
				return subPath
			}
		}
	}
	return ""
}

func (p *Provider) isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func (p *Provider) addWorktree(repoPath, worktreePath, branch string) error {
	args := []string{"-C", repoPath, "worktree", "add", worktreePath, "-b", branch}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			delCmd := exec.Command("git", "-C", repoPath, "branch", "-D", branch)
			if delOut, delErr := delCmd.CombinedOutput(); delErr != nil {
				return fmt.Errorf("git worktree add failed (branch exists, delete failed): %w\nDelete output: %s\nOriginal output: %s",
					delErr, string(delOut), string(output))
			}
			retryCmd := exec.Command("git", args...)
			if retryOut, retryErr := retryCmd.CombinedOutput(); retryErr != nil {
				return fmt.Errorf("git worktree add failed on retry: %w\nOutput: %s", retryErr, string(retryOut))
			}
			return nil
		}
		return fmt.Errorf("git worktree add failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (p *Provider) removeWorktree(repoPath, worktreePath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// populateMergedDir creates symlinks in mergedDir pointing to each top-level
// entry from all lower directories (fallback when no git repo is present).
func (p *Provider) populateMergedDir(lowerDirs []string, mergedDir string) error {
	for i := len(lowerDirs) - 1; i >= 0; i-- {
		dir := lowerDirs[i]
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("failed to read lower dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			linkPath := filepath.Join(mergedDir, name)
			targetPath := filepath.Join(dir, name)
			if resolved, err := filepath.EvalSymlinks(targetPath); err == nil {
				targetPath = resolved
			}
			if _, err := os.Lstat(linkPath); err == nil {
				if err := os.Remove(linkPath); err != nil {
					return fmt.Errorf("failed to remove existing entry %s: %w", linkPath, err)
				}
			}
			if err := os.Symlink(targetPath, linkPath); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, targetPath, err)
			}
		}
	}
	return nil
}

func (p *Provider) createDirectories(upperDir, workDir, mergedDir string) error {
	for _, dir := range []string{upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) cleanupDirectories(upperDir, workDir, mergedDir string) error {
	for _, dir := range []string{mergedDir, workDir, upperDir} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) cleanup(upperDir, workDir, mergedDir string) error {
	if err := p.cleanupDirectories(upperDir, workDir, mergedDir); err != nil {
		return sandbox.NewCleanupFailedError(mergedDir, err)
	}
	return nil
}

// writeSecrets writes secrets to upperdir/.env file. Values are passed through
// os.ExpandEnv so callers can reference host environment variables.
func (p *Provider) writeSecrets(upperDir string, secrets map[string]string) error {
	envFile := filepath.Join(upperDir, ".env")

	var buf strings.Builder
	buf.WriteString("# Auto-generated by AGM sandbox\n")
	buf.WriteString("# DO NOT COMMIT THIS FILE\n\n")
	for key, value := range secrets {
		expandedValue := os.ExpandEnv(value)
		fmt.Fprintf(&buf, "%s=%s\n", key, expandedValue)
	}
	if err := os.WriteFile(envFile, []byte(buf.String()), 0600); err != nil {
		return err
	}
	return nil
}
