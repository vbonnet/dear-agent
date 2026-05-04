// Package builtin provides builtin-related functionality.
package builtin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/engram/hooks"
)

// CleanupChecker detects cleanup tasks needed
type CleanupChecker struct {
	projectRoot string
}

// NewCleanupChecker creates a new cleanup checker
func NewCleanupChecker(projectRoot string) *CleanupChecker {
	return &CleanupChecker{
		projectRoot: projectRoot,
	}
}

// CheckCleanup detects temporary files and merged branches
func (cc *CleanupChecker) CheckCleanup(ctx context.Context) (*hooks.VerificationResult, error) {
	var violations []hooks.Violation

	// Check for temporary files
	tempFiles := cc.detectTempFiles()
	if len(tempFiles) > 0 {
		violations = append(violations, hooks.Violation{
			Severity:   "low",
			Message:    fmt.Sprintf("Found %d temporary files", len(tempFiles)),
			Files:      tempFiles,
			Suggestion: "Remove temporary files: " + strings.Join(tempFiles, ", "),
		})
	}

	// Check for merged branches
	mergedBranches, err := cc.detectMergedBranches(ctx)
	if err == nil && len(mergedBranches) > 0 {
		violations = append(violations, hooks.Violation{
			Severity:   "low",
			Message:    fmt.Sprintf("Found %d merged branches that can be deleted", len(mergedBranches)),
			Suggestion: "Delete merged branches: git branch -d " + strings.Join(mergedBranches, " "),
		})
	}

	// Check for unused worktrees
	unusedWorktrees, err := cc.detectUnusedWorktrees(ctx)
	if err == nil && len(unusedWorktrees) > 0 {
		violations = append(violations, hooks.Violation{
			Severity:   "low",
			Message:    fmt.Sprintf("Found %d potentially unused worktrees", len(unusedWorktrees)),
			Suggestion: "Review and remove unused worktrees: git worktree remove <path>",
		})
	}

	status := hooks.VerificationStatusPass
	if len(violations) > 0 {
		status = hooks.VerificationStatusWarning
	}

	return &hooks.VerificationResult{
		Status:     status,
		Violations: violations,
	}, nil
}

// detectTempFiles finds temporary files in the project
func (cc *CleanupChecker) detectTempFiles() []string {
	var tempFiles []string

	tempPatterns := []string{
		"*.backup",
		"*-draft.md",
		"*.tmp",
		"*.swp",
		".DS_Store",
		"*~",
	}

	for _, pattern := range tempPatterns {
		matches, err := filepath.Glob(filepath.Join(cc.projectRoot, pattern))
		if err != nil {
			continue
		}

		// Make paths relative to project root
		for _, match := range matches {
			relPath, err := filepath.Rel(cc.projectRoot, match)
			if err == nil {
				tempFiles = append(tempFiles, relPath)
			}
		}
	}

	return tempFiles
}

// detectMergedBranches finds branches that have been merged
func (cc *CleanupChecker) detectMergedBranches(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--merged")
	cmd.Dir = cc.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var branches []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		// Skip main/master branches
		if branch != "" && branch != "main" && branch != "master" {
			branches = append(branches, branch)
		}
	}

	return branches, nil
}

// detectUnusedWorktrees finds potentially unused git worktrees
func (cc *CleanupChecker) detectUnusedWorktrees(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list")
	cmd.Dir = cc.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var worktrees []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Skip the first line (main worktree)
	for i, line := range lines {
		if i == 0 {
			continue
		}

		// Parse worktree path (first field)
		parts := strings.Fields(line)
		if len(parts) > 0 {
			// Check if worktree still exists
			wtPath := parts[0]
			if _, err := os.Stat(wtPath); err == nil {
				worktrees = append(worktrees, wtPath)
			}
		}
	}

	return worktrees, nil
}
