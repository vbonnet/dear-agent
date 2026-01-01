// Package git provides interfaces and implementations for git repository operations.
//
// It supports bare repositories with worktree management, which is the recommended
// approach for parallel development with multiple feature branches.
package git

// Repository represents a git repository with worktree operations.
// The primary implementation uses bare repositories for efficient worktree management.
type Repository interface {
	// Clone creates a bare repository at the given path from the specified URL.
	// For bare repos, use: git clone --bare <url> <path>
	Clone(url, path string) error

	// CreateWorktree creates a new worktree with the given name and branch.
	// If the branch doesn't exist, it will be created tracking origin/<branch>.
	CreateWorktree(name, branch string) error

	// ListWorktrees returns all worktrees in the repository.
	// For bare repos, this lists all git worktrees.
	ListWorktrees() ([]WorktreeInfo, error)

	// GetCurrentBranch returns the current branch for a worktree.
	// Returns empty string if worktree doesn't exist or is detached.
	GetCurrentBranch(worktree string) (string, error)

	// Exists checks if the repository exists at the configured path.
	// For bare repos, checks if the repository directory structure exists
	// (bare repos use HEAD, objects/, refs/ instead of a .git directory).
	Exists() bool
}

// WorktreeInfo contains information about a git worktree.
type WorktreeInfo struct {
	Name   string // Worktree name (directory name)
	Path   string // Full path to worktree
	Branch string // Current branch
	Commit string // Current commit SHA
}
