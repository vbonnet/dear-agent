// Package worktree provides session-based worktree isolation for multi-agent collaboration.
// It detects repository structures, manages session-specific worktrees, and redirects file paths
// to prevent conflicts between parallel agents working on the same repository.
package worktree

import (
	"os"
	"path/filepath"
)

// RepoStructure represents the organizational pattern of a git repository.
type RepoStructure int

const (
	// StructureStandard represents a traditional git repository with .git directory at root.
	// Worktrees are created in a global worktree base (e.g., ~/worktrees).
	StructureStandard RepoStructure = iota

	// StructureBare represents a bare repository structure with .bare/ directory.
	// Worktrees are created within .bare/worktrees/ (when implemented).
	// Note: This is currently aspirational - actual repos use StructureStandard.
	StructureBare
)

// DetectStructure determines the organizational pattern of a git repository.
// It checks for the presence of a .bare/ directory to distinguish between
// standard and bare repository structures.
//
// Parameters:
//   - repoPath: Absolute path to the repository root
//
// Returns:
//   - RepoStructure: The detected structure type
//   - error: Any error encountered during detection
//
// Example:
//
//	structure, err := DetectStructure("/tmp/test/src/myrepo")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if structure == StructureBare {
//	    fmt.Println("Using bare repository structure")
//	}
func DetectStructure(repoPath string) (RepoStructure, error) {
	// Check for .bare/ directory (future bare repo implementation)
	barePath := filepath.Join(repoPath, ".bare")
	info, err := os.Stat(barePath)
	if err == nil && info.IsDir() {
		return StructureBare, nil
	}

	// Default to standard structure (current implementation)
	// Standard repos have .git at root and worktrees in ~/worktrees
	return StructureStandard, nil
}

// GetWorktreeBase returns the appropriate base directory for worktrees
// based on the repository structure.
//
// For standard repositories, worktrees are created in a global location
// (default: ~/worktrees) to avoid cluttering the repository directory.
//
// For bare repositories, worktrees are created within the repository's
// .bare/worktrees/ directory for better organization.
//
// Parameters:
//   - structure: The repository structure type
//   - repoPath: Absolute path to the repository root
//
// Returns:
//   - string: Absolute path to the worktree base directory
//
// Example:
//
//	structure, _ := DetectStructure("/tmp/test/src/myrepo")
//	base := GetWorktreeBase(structure, "/tmp/test/src/myrepo")
//	// Standard: returns "/tmp/test/worktrees"
//	// Bare: returns "/tmp/test/src/myrepo/.bare/worktrees"
func GetWorktreeBase(structure RepoStructure, repoPath string) string {
	switch structure {
	case StructureStandard:
		// Standard repos: worktrees in global directory
		// Use expandHome to handle ~ prefix
		return expandHome("~/worktrees")
	case StructureBare:
		// Bare repos: worktrees live inside .bare/worktrees/
		return filepath.Join(repoPath, ".bare", "worktrees")
	default:
		// Unknown structure: fall back to standard behavior
		return expandHome("~/worktrees")
	}
}

// expandHome replaces a leading ~ with the user's home directory.
// Returns the path unchanged if it doesn't start with ~ or if home cannot be determined.
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if len(path) == 1 {
		return home
	}
	return filepath.Join(home, path[1:])
}
