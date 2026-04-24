// Package git provides interfaces and implementations for git repository operations.
package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	deverrors "github.com/vbonnet/dear-agent/tools/devlog/internal/errors"
)

// LocalRepository implements Repository using local git commands.
// It operates on bare repositories for efficient worktree management.
type LocalRepository struct {
	Path string // Absolute path to the bare repository
}

// NewLocalRepository creates a new LocalRepository instance.
// The path should point to a bare git repository directory.
func NewLocalRepository(path string) *LocalRepository {
	return &LocalRepository{Path: path}
}

// Clone creates a bare repository at the configured path from the specified URL.
// Uses: git clone --bare <url> <path>
func (r *LocalRepository) Clone(url, path string) error {
	// Security: Use exec.CommandContext with separate arguments to prevent command injection
	cmd := exec.CommandContext(context.TODO(), "git", "clone", "--bare", url, path) //nolint:gosec // git path from config
	output, err := cmd.CombinedOutput()
	if err != nil {
		return deverrors.WrapPath("clone repository", path,
			fmt.Errorf("git clone failed: %w (output: %s)", err, strings.TrimSpace(string(output))))
	}
	return nil
}

// CreateWorktree creates a new worktree with the given name and branch.
// If the branch doesn't exist locally, it will be created tracking origin/<branch>.
// Uses: git worktree add <name> <branch>
func (r *LocalRepository) CreateWorktree(name, branch string) error {
	if !r.Exists() {
		return deverrors.WrapPath("create worktree", r.Path, deverrors.ErrGitFailed)
	}

	worktreePath := filepath.Join(r.Path, name)

	// Security: Use exec.Command with separate arguments to prevent command injection
	cmd := exec.Command("git", "-C", r.Path, "worktree", "add", worktreePath, branch) //nolint:gosec // git path from config
	output, err := cmd.CombinedOutput()
	if err != nil {
		return deverrors.WrapPath("create worktree", worktreePath,
			fmt.Errorf("git worktree add failed: %w (output: %s)", err, strings.TrimSpace(string(output))))
	}
	return nil
}

// ListWorktrees returns all worktrees in the repository.
// Uses: git worktree list --porcelain
func (r *LocalRepository) ListWorktrees() ([]WorktreeInfo, error) {
	if !r.Exists() {
		return nil, deverrors.WrapPath("list worktrees", r.Path, deverrors.ErrGitFailed)
	}

	cmd := exec.Command("git", "-C", r.Path, "worktree", "list", "--porcelain") //nolint:gosec // git path from config
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, deverrors.WrapPath("list worktrees", r.Path,
			fmt.Errorf("git worktree list failed: %w", err))
	}

	return parseWorktreeList(string(output))
}

// GetCurrentBranch returns the current branch for a worktree.
// Returns empty string if worktree doesn't exist or is detached.
// Uses: git -C <worktree> branch --show-current
func (r *LocalRepository) GetCurrentBranch(worktree string) (string, error) {
	worktreePath := filepath.Join(r.Path, worktree)

	// Check if worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return "", nil
	}

	cmd := exec.CommandContext(context.TODO(), "git", "-C", worktreePath, "branch", "--show-current")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Detached HEAD state returns error, return empty string
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// Exists checks if the repository exists at the configured path.
// For bare repos, checks if HEAD, objects/, and refs/ exist.
func (r *LocalRepository) Exists() bool {
	// Check for bare repo structure: HEAD, objects/, refs/
	headPath := filepath.Join(r.Path, "HEAD")
	objectsPath := filepath.Join(r.Path, "objects")
	refsPath := filepath.Join(r.Path, "refs")

	_, err1 := os.Stat(headPath)
	info2, err2 := os.Stat(objectsPath)
	info3, err3 := os.Stat(refsPath)

	return err1 == nil && err2 == nil && err3 == nil &&
		info2.IsDir() && info3.IsDir()
}

// parseWorktreeList parses the output of `git worktree list --porcelain`.
// Format:
//
//	worktree /path/to/worktree
//	HEAD abcdef1234567890
//	branch refs/heads/main
//
//	worktree /path/to/other
//	HEAD 1234567890abcdef
//	detached
func parseWorktreeList(output string) ([]WorktreeInfo, error) {
	var worktrees []WorktreeInfo
	lines := strings.Split(output, "\n")

	var current *WorktreeInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line separates worktrees
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			// Handle single-word lines like "detached"
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case "worktree":
			current = &WorktreeInfo{
				Path: value,
				Name: filepath.Base(value),
			}
		case "HEAD":
			if current != nil {
				current.Commit = value
			}
		case "branch":
			if current != nil {
				// value is like "refs/heads/main", extract "main"
				branch := strings.TrimPrefix(value, "refs/heads/")
				current.Branch = branch
			}
		}
	}

	// Don't forget the last worktree
	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees, nil
}
