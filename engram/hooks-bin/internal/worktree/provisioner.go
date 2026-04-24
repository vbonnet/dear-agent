package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ProvisionerConfig holds the configuration for worktree provisioning.
type ProvisionerConfig struct {
	WorktreeBase   string // Base directory for worktrees (e.g., ~/worktrees or .bare/worktrees)
	RepositoryRoot string // Absolute path to the repository root
	SessionID      string // Current session ID for unique worktree naming
	BranchName     string // Optional: specific branch name (defaults to session-<id>)
}

// Provisioner creates and manages session-specific git worktrees.
// It provides idempotent worktree creation with caching to minimize
// filesystem operations and git command overhead.
type Provisioner struct {
	config *ProvisionerConfig
	cache  *ProvisionCache
}

// NewProvisioner creates a new Provisioner with the given configuration.
func NewProvisioner(config *ProvisionerConfig, cache *ProvisionCache) *Provisioner {
	return &Provisioner{
		config: config,
		cache:  cache,
	}
}

// Provision creates a new git worktree for the session if it doesn't already exist.
// This operation is idempotent - calling it multiple times for the same session
// returns the same worktree path without creating duplicates.
//
// The provisioning process:
//  1. Check in-memory cache for existing worktree
//  2. Check filesystem for existing worktree directory
//  3. Create new worktree with atomic git operation if needed
//  4. Cache the result for future calls
//
// Returns:
//   - string: Absolute path to the session worktree
//   - error: Any error encountered during provisioning
//
// Performance:
//   - Cache hit: <1ms
//   - Cache miss (worktree exists): ~10ms (filesystem check)
//   - First provision: ~80ms (git worktree add command)
func (p *Provisioner) Provision() (string, error) {
	// Check cache first (fastest path)
	if cached := p.cache.Get(p.config.SessionID); cached != "" {
		return cached, nil
	}

	// Check if worktree already exists on filesystem
	worktreePath := p.GetPath()
	if p.Exists() {
		// Worktree exists but wasn't cached - add to cache
		p.cache.Set(p.config.SessionID, worktreePath)
		return worktreePath, nil
	}

	// Create new worktree atomically
	branchName := p.config.BranchName
	if branchName == "" {
		branchName = FormatWorktreeName(p.config.SessionID)
	}

	// Ensure worktree base directory exists
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0750); err != nil {
		return "", fmt.Errorf("failed to create worktree base directory: %w", err)
	}

	// Execute git worktree add
	cmd := exec.CommandContext(context.Background(), "git", "-C", p.config.RepositoryRoot, //nolint:gosec // args from trusted config
		"worktree", "add", worktreePath, "-b", branchName)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git worktree add failed: %w: %s", err, output)
	}

	// Cache the newly created worktree
	p.cache.Set(p.config.SessionID, worktreePath)

	return worktreePath, nil
}

// Exists checks if the session worktree directory already exists on the filesystem.
// This is used to detect worktrees that were created in previous hook invocations
// but aren't in the cache (cache is per-process, not persistent).
//
// Returns:
//   - bool: true if worktree directory exists
func (p *Provisioner) Exists() bool {
	worktreePath := p.GetPath()
	info, err := os.Stat(worktreePath)
	return err == nil && info.IsDir()
}

// GetPath returns the expected absolute path to the session worktree.
// The path is deterministic based on session ID, allowing multiple hook
// invocations to reference the same worktree.
//
// Path format:
//   - Standard repos: ~/worktrees/session-<id>/
//   - Bare repos: <repo>/.bare/worktrees/session-<id>/
//
// Returns:
//   - string: Absolute path to the session worktree directory
func (p *Provisioner) GetPath() string {
	worktreeName := FormatWorktreeName(p.config.SessionID)
	return filepath.Join(p.config.WorktreeBase, worktreeName)
}

// GetBranchName returns the git branch name for the session worktree.
// Returns the configured branch name if set, otherwise generates
// a default name based on the session ID.
func (p *Provisioner) GetBranchName() string {
	if p.config.BranchName != "" {
		return p.config.BranchName
	}
	return FormatWorktreeName(p.config.SessionID)
}
