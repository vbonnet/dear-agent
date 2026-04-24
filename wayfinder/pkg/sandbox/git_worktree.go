package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitWorktreeManager manages git worktree operations
type GitWorktreeManager struct{}

// NewGitWorktreeManager creates a new git worktree manager
func NewGitWorktreeManager() *GitWorktreeManager {
	return &GitWorktreeManager{}
}

// IsGitRepository checks if path is within a git repository
func (g *GitWorktreeManager) IsGitRepository(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// GetRepositoryRoot returns the root directory of the git repository
func (g *GitWorktreeManager) GetRepositoryRoot(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetGitVersion returns the installed git version
func (g *GitWorktreeManager) GetGitVersion() (string, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Parse "git version 2.34.1" -> "2.34.1"
	parts := strings.Fields(string(output))
	if len(parts) >= 3 {
		return parts[2], nil
	}
	return "", fmt.Errorf("unexpected git version output: %s", output)
}

// CheckGitVersion checks if git version meets minimum requirement (2.5.0)
func (g *GitWorktreeManager) CheckGitVersion() error {
	version, err := g.GetGitVersion()
	if err != nil {
		return fmt.Errorf("failed to get git version: %w", err)
	}

	// Simple version check (assumes semantic versioning)
	// TODO: Implement proper version comparison
	if strings.HasPrefix(version, "1.") || strings.HasPrefix(version, "2.0.") ||
		strings.HasPrefix(version, "2.1.") || strings.HasPrefix(version, "2.2.") ||
		strings.HasPrefix(version, "2.3.") || strings.HasPrefix(version, "2.4.") {
		return fmt.Errorf("git 2.5+ required, found %s", version)
	}

	return nil
}

// CreateWorktree creates a new git worktree for the sandbox
func (g *GitWorktreeManager) CreateWorktree(sandboxID, repoRoot, branch string) (string, error) {
	// Check git version
	if err := g.CheckGitVersion(); err != nil {
		return "", err
	}

	// Create worktree directory
	worktreePath := filepath.Join(repoRoot, ".worktrees", sandboxID)

	// Ensure .worktrees directory exists
	if err := os.MkdirAll(filepath.Join(repoRoot, ".worktrees"), 0755); err != nil {
		return "", fmt.Errorf("failed to create .worktrees directory: %w", err)
	}

	// Create worktree
	args := []string{"-C", repoRoot, "worktree", "add", worktreePath}
	if branch != "" {
		args = append(args, branch)
	}

	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git worktree add failed: %w: %s", err, stderr.String())
	}

	return worktreePath, nil
}

// RemoveWorktree removes a git worktree
func (g *GitWorktreeManager) RemoveWorktree(sandboxID, repoRoot string) error {
	worktreePath := filepath.Join(repoRoot, ".worktrees", sandboxID)

	// Check if worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		// Already removed
		return nil
	}

	// Remove worktree using git command
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "remove", worktreePath, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If git command fails, try manual cleanup
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("failed to remove worktree directory: %w", err)
		}

		// Prune worktree references
		pruneCmd := exec.Command("git", "-C", repoRoot, "worktree", "prune")
		_ = pruneCmd.Run() // Ignore errors
	}

	return nil
}

// ListWorktrees returns all worktrees in the repository
func (g *GitWorktreeManager) ListWorktrees(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var worktrees []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			worktrees = append(worktrees, path)
		}
	}

	return worktrees, nil
}
