// Package workspace provides workspace management for devlog.
//
// A workspace represents a development environment with multiple git repositories
// configured via YAML files in the .devlog directory.
package workspace

import (
	"path/filepath"

	"github.com/vbonnet/dear-agent/tools/devlog/internal/config"
)

// Workspace represents a devlog workspace with repos and worktrees.
type Workspace struct {
	Config *config.Config // Loaded configuration
	Root   string         // Workspace root directory (where .devlog/ is located)
}

// LoadWorkspace discovers and loads workspace config from a starting path.
// It walks up the directory tree to find .devlog/config.yaml.
func LoadWorkspace(startPath string) (*Workspace, error) {
	// Use existing config loading logic
	cfg, err := config.LoadMerged(startPath)
	if err != nil {
		return nil, err
	}

	// Determine workspace root (directory containing .devlog/)
	// For now, use startPath; could enhance to find actual root
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, err
	}

	return &Workspace{
		Config: cfg,
		Root:   absPath,
	}, nil
}

// GetRepoPath returns the full path for a repository.
// By default, repos are stored in the workspace root directory.
func (w *Workspace) GetRepoPath(repo *config.Repo) string {
	return filepath.Join(w.Root, repo.Name)
}

// GetWorktreePath returns the full path for a worktree.
// Worktrees are stored as subdirectories of the repository.
func (w *Workspace) GetWorktreePath(repo *config.Repo, worktree *config.Worktree) string {
	repoPath := w.GetRepoPath(repo)
	return filepath.Join(repoPath, worktree.Name)
}
