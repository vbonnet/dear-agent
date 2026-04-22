package worktree

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Redirector handles path redirection from shared worktrees to session-specific worktrees.
// It determines when redirection is needed and calculates the target path while preserving
// the relative directory structure.
type Redirector struct {
	provisioner *Provisioner
}

// NewRedirector creates a new Redirector with the given provisioner.
func NewRedirector(provisioner *Provisioner) *Redirector {
	return &Redirector{
		provisioner: provisioner,
	}
}

// RedirectResult contains the outcome of a redirection check.
type RedirectResult struct {
	ShouldRedirect  bool   // True if the path should be redirected
	RedirectedPath  string // The target path in the session worktree (if redirecting)
	SessionWorktree string // Absolute path to the session worktree directory
	Provisioned     bool   // True if the worktree was created during this operation
}

// RedirectIfNeeded checks if a file path requires redirection and provisions
// a session worktree if necessary.
//
// Redirection is triggered when:
//  1. The operation is a write operation (Write/Edit/MultiEdit)
//  2. The path is in a shared worktree (main/ or base/)
//
// The redirection process:
//  1. Detect if path is in a shared location
//  2. Provision session worktree (idempotent)
//  3. Calculate redirected path preserving relative structure
//
// Parameters:
//   - filePath: The file path being written to
//   - toolName: The tool being invoked (Write, Edit, MultiEdit, etc.)
//
// Returns:
//   - *RedirectResult: The redirection decision and paths
//   - error: Any error encountered during redirection
//
// Example:
//
//	redirector := NewRedirector(provisioner)
//	result, err := redirector.RedirectIfNeeded("/repo/main/src/file.go", "Write")
//	if err != nil {
//	    return err
//	}
//	if result.ShouldRedirect {
//	    // Use result.RedirectedPath instead of original path
//	    fmt.Printf("Redirected to: %s\n", result.RedirectedPath)
//	}
func (r *Redirector) RedirectIfNeeded(filePath string, toolName string) (*RedirectResult, error) {
	// Only redirect write operations
	if !isWriteOperation(toolName) {
		return &RedirectResult{ShouldRedirect: false}, nil
	}

	// Detect if in shared worktree (main/base) or main repo
	isShared, err := r.isInSharedLocation(filePath)
	if err != nil || !isShared {
		return &RedirectResult{ShouldRedirect: false}, err
	}

	// Check if worktree was already provisioned
	wasProvisioned := r.provisioner.Exists()

	// Provision session worktree if needed
	sessionWorktree, err := r.provisioner.Provision()
	if err != nil {
		return nil, fmt.Errorf("provisioning failed: %w", err)
	}

	// Calculate redirected path
	redirectedPath, err := r.calculateRedirectedPath(filePath, sessionWorktree)
	if err != nil {
		return nil, err
	}

	return &RedirectResult{
		ShouldRedirect:  true,
		RedirectedPath:  redirectedPath,
		SessionWorktree: sessionWorktree,
		Provisioned:     !wasProvisioned,
	}, nil
}

// isInSharedLocation detects if a path is in a main/base worktree or main repository.
// These are considered "shared" locations where direct writes should be prevented
// to avoid conflicts between parallel agents.
//
// Shared location patterns:
//   - Paths containing /main/ segment (e.g., /repo/main/file.go)
//   - Paths containing /base/ segment (e.g., /repo/base/file.go)
//   - Paths NOT containing /worktrees/ segment (main repository)
//
// Returns:
//   - bool: true if path is in a shared location
//   - error: Any error during detection
func (r *Redirector) isInSharedLocation(filePath string) (bool, error) {
	// Resolve to absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false, err
	}

	// Normalize path separators
	absPath = filepath.Clean(absPath)

	// Check for shared worktree patterns
	// Pattern 1: Contains /main/ directory segment
	if strings.Contains(absPath, string(filepath.Separator)+"main"+string(filepath.Separator)) {
		return true, nil
	}

	// Pattern 2: Contains /base/ directory segment
	if strings.Contains(absPath, string(filepath.Separator)+"base"+string(filepath.Separator)) {
		return true, nil
	}

	// Pattern 3: NOT in any worktree (main repository)
	// Worktrees are typically in paths containing /worktrees/
	if !strings.Contains(absPath, string(filepath.Separator)+"worktrees"+string(filepath.Separator)) {
		// This is the main repository - check if it's actually a git repo
		isGitRepo, err := r.isGitRepository(absPath)
		if err != nil {
			return false, nil //nolint:nilerr // Not a git repo, don't redirect
		}
		return isGitRepo, nil
	}

	return false, nil
}

// isGitRepository checks if a file path is within a git repository.
// This is used to distinguish main repositories from non-git paths.
func (r *Redirector) isGitRepository(filePath string) (bool, error) { //nolint:unparam // error kept for interface consistency
	dir := filepath.Dir(filePath)

	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--git-dir")
	err := cmd.Run()

	// If git command succeeds, it's a git repository
	return err == nil, nil
}

// calculateRedirectedPath computes the target path in the session worktree
// while preserving the relative directory structure from the repository root.
//
// Example:
//
//	Original: /repo/main/src/pkg/file.go
//	Worktree: /worktrees/session-abc123
//	Result:   /worktrees/session-abc123/src/pkg/file.go
//
// Parameters:
//   - original: Original file path (in main/base worktree or main repo)
//   - worktree: Absolute path to session worktree directory
//
// Returns:
//   - string: Redirected path in session worktree
//   - error: Any error during path calculation
func (r *Redirector) calculateRedirectedPath(original, worktree string) (string, error) {
	// Get repository root
	cmd := exec.CommandContext(context.Background(), "git", "-C", filepath.Dir(original), "rev-parse", "--show-toplevel") //nolint:gosec // path from trusted input
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find repository root: %w", err)
	}

	repoRoot := strings.TrimSpace(string(output))

	// Resolve symlinks to canonical paths (important for macOS: /tmp -> /private/tmp)
	realRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		realRoot = repoRoot
	}

	realOriginal, err := filepath.EvalSymlinks(original)
	if err != nil {
		realOriginal = original
	}

	realWorktree, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		realWorktree = worktree
	}

	// Calculate relative path from repo root
	relPath, err := filepath.Rel(realRoot, realOriginal)
	if err != nil {
		return "", fmt.Errorf("failed to calculate relative path: %w", err)
	}

	// Join with session worktree path
	return filepath.Join(realWorktree, relPath), nil
}

// isWriteOperation returns true if the tool modifies files (subject to enforcement).
func isWriteOperation(toolName string) bool {
	writeTools := map[string]bool{
		"Write":     true,
		"Edit":      true,
		"MultiEdit": true,
	}
	return writeTools[toolName]
}
