// Package git provides git functionality.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CommitManifest automatically commits a manifest file if the sessions directory
// is within a git repository. This function is safe to call in non-git directories
// (it will silently return without error).
//
// Requirements:
// - Only commits the specific manifest file provided
// - Works correctly even if the repo has other unstaged or staged files
// - Non-invasive: returns early if not in a git repo
// - Graceful error handling with descriptive messages
//
// Parameters:
//   - manifestPath: Absolute path to the manifest.yaml file to commit
//   - operation: Description of the operation (e.g., "create", "archive", "associate")
//   - sessionName: Name of the session for the commit message
//
// Returns:
//   - error: nil on success, error on failure (or nil if not in git repo)
func CommitManifest(manifestPath, operation, sessionName string) error {
	// Get the directory containing the manifest (should be sessions dir or subdirectory)
	manifestDir := filepath.Dir(manifestPath)

	// Check if we're in a git repository by looking for .git directory
	// Walk up from manifest directory to find git root
	gitRoot, err := findGitRoot(manifestDir)
	if err != nil {
		// Not in a git repo - this is OK, just return silently
		return nil
	}

	// Verify the manifest file exists
	if _, err := os.Stat(manifestPath); err != nil {
		// File doesn't exist - this might be an error in the caller
		// But we don't want to fail here, so return nil
		return nil
	}

	// Make manifest path relative to git root for cleaner git commands
	relPath, err := filepath.Rel(gitRoot, manifestPath)
	if err != nil {
		// Can't compute relative path - use absolute path
		relPath = manifestPath
	}

	// Add the manifest file to staging area
	// This is required for both new and modified files
	addCmd := exec.Command("git", "-C", gitRoot, "add", relPath)
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage manifest file: %w\nOutput: %s", err, string(output))
	}

	// Create commit message
	commitMsg := fmt.Sprintf("agm: %s session '%s'", operation, sessionName)

	// Commit only the manifest file using --only flag
	// This commits only the specified file, even if other files are staged
	// Note: --only requires the file to be added first (done above)
	commitCmd := exec.Command("git", "-C", gitRoot, "commit", "--only", relPath, "-m", commitMsg)
	if output, err := commitCmd.CombinedOutput(); err != nil {
		// Check if the error is because there's nothing to commit
		// This can happen if the manifest content didn't actually change
		if strings.Contains(string(output), "nothing to commit") ||
			strings.Contains(string(output), "no changes added to commit") {
			// This is fine - no changes to commit
			return nil
		}
		return fmt.Errorf("failed to commit manifest: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// findGitRoot walks up the directory tree from the given path to find
// the root of a git repository (directory containing .git).
//
// Returns:
//   - string: absolute path to the git root directory
//   - error: ErrNotInGitRepo if no .git directory is found
func findGitRoot(startPath string) (string, error) {
	// Ensure we have an absolute path
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Walk up the directory tree
	currentPath := absPath
	for {
		// Check if .git exists in current directory
		gitPath := filepath.Join(currentPath, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// .git exists - check if it's a directory or file (for worktrees)
			if info.IsDir() {
				// Regular git repo
				return currentPath, nil
			}
			// .git file (git worktree) - still a valid git repo
			return currentPath, nil
		}

		// Move up one directory
		parentPath := filepath.Dir(currentPath)

		// Check if we've reached the root
		if parentPath == currentPath {
			// We've reached the filesystem root without finding .git
			return "", ErrNotInGitRepo
		}

		currentPath = parentPath
	}
}

// ErrNotInGitRepo is returned when no git repository is found in the directory tree
var ErrNotInGitRepo = fmt.Errorf("not in a git repository")

// IsInGitRepo checks if the given path is within a git repository.
// This is a convenience function that wraps findGitRoot.
func IsInGitRepo(path string) bool {
	_, err := findGitRoot(path)
	return err == nil
}
