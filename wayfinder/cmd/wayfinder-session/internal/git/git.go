// Package git provides git-related functionality.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitIntegrator handles git operations for wayfinder sessions
type GitIntegrator struct {
	projectDir string
}

// New creates a new GitIntegrator for the given project directory
func New(projectDir string) *GitIntegrator {
	return &GitIntegrator{
		projectDir: projectDir,
	}
}

// IsGitRepo checks if the project directory is within a git repository
// Works correctly even when project is a subdirectory of a git repo
func (g *GitIntegrator) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = g.projectDir
	err := cmd.Run()
	return err == nil
}

// CommitPhaseCompletion creates a git commit for phase completion
// Adds WAYFINDER-STATUS.md and WAYFINDER-HISTORY.md to staging and commits
func (g *GitIntegrator) CommitPhaseCompletion(phase, outcome, context string) error {
	if !g.IsGitRepo() {
		return fmt.Errorf("project directory is not a git repository")
	}

	// Check if working directory is clean (ignoring wayfinder files)
	// We allow wayfinder files to be uncommitted
	statusFiles := []string{
		"WAYFINDER-STATUS.md",
		"WAYFINDER-HISTORY.md",
	}

	// Add wayfinder files to staging
	for _, file := range statusFiles {
		filePath := filepath.Join(g.projectDir, file)
		if _, err := os.Stat(filePath); err == nil {
			// File exists, add it
			if err := g.gitAdd(file); err != nil {
				return fmt.Errorf("failed to add %s: %w", file, err)
			}
		}
	}

	// Create commit message
	commitMsg := g.formatCommitMessage(phase, outcome, context)

	// Create commit
	if err := g.gitCommit(commitMsg); err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}

// formatCommitMessage creates a standardized commit message for phase completion
func (g *GitIntegrator) formatCommitMessage(phase, outcome, context string) string {
	var msg strings.Builder

	// Subject line
	fmt.Fprintf(&msg, "wayfinder: complete %s (%s)", phase, outcome)

	// Add context if provided
	if context != "" {
		msg.WriteString("\n\n")
		msg.WriteString(context)
	}

	// Add metadata footer
	msg.WriteString("\n\n")
	msg.WriteString("Wayfinder-Phase: " + phase + "\n")
	msg.WriteString("Wayfinder-Outcome: " + outcome)

	return msg.String()
}

// gitAdd runs git add for a file
func (g *GitIntegrator) gitAdd(file string) error {
	cmd := exec.Command("git", "add", file)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// gitCommit creates a git commit with the given message
func (g *GitIntegrator) gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if error is "nothing to commit"
		if strings.Contains(string(output), "nothing to commit") {
			return nil // Not an error - nothing changed
		}
		return fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// GetCommitHash returns the current HEAD commit hash
func (g *GitIntegrator) GetCommitHash() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// sourceCodeExtensions defines file extensions that are considered source code
var sourceCodeExtensions = map[string]bool{
	".go":   true, // Go
	".py":   true, // Python
	".js":   true, // JavaScript
	".ts":   true, // TypeScript
	".jsx":  true, // React JavaScript
	".tsx":  true, // React TypeScript
	".c":    true, // C
	".cpp":  true, // C++
	".java": true, // Java
	".rb":   true, // Ruby
	".rs":   true, // Rust
	".php":  true, // PHP
}

// isSourceCodeFile returns true if file extension matches source code allowlist
func isSourceCodeFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	return sourceCodeExtensions[ext]
}

// isInProjectDir returns true if file is under project directory
func isInProjectDir(filePath, projectDir string) bool {
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return false // Conservative: if can't resolve, assume NOT in project-dir
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return false
	}

	// Add separator to avoid false matches (e.g., ~/project-dir-2 matching ~/project-dir)
	return strings.HasPrefix(absFilePath, absProjectDir+string(filepath.Separator))
}

// GetUncommittedFilesInProjectDir returns list of uncommitted/unstaged files
// within the project directory, excluding .wayfinder/ internal metadata.
//
// Algorithm:
//  1. Run: git status --porcelain . (from projectDir context)
//  2. Parse output for ANY modified/unstaged files
//  3. Filter out .wayfinder/ directory paths
//  4. Return remaining files
//
// Returns empty list if:
//   - No git repo found (not an error)
//   - No uncommitted files in project directory
//   - Only .wayfinder/ files are uncommitted
func (g *GitIntegrator) GetUncommittedFilesInProjectDir() ([]string, error) {
	// Check if git repo exists
	if !g.IsGitRepo() {
		return []string{}, nil // Not an error - return empty list
	}

	// Execute git status --porcelain . to check only project directory
	cmd := exec.Command("git", "status", "--porcelain", ".")
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w (output: %s)", err, string(output))
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var uncommittedFiles []string

	for _, line := range lines {
		if len(line) < 3 {
			continue // Skip empty lines
		}

		// All non-empty status lines indicate uncommitted/unstaged files
		statusCode := line[0:2]
		filePath := strings.TrimSpace(line[3:])

		// Skip ignored files
		if statusCode == "!!" {
			continue
		}

		// Skip .wayfinder/ internal directory
		if strings.HasPrefix(filePath, ".wayfinder/") || strings.Contains(filePath, "/.wayfinder/") {
			continue
		}

		uncommittedFiles = append(uncommittedFiles, filePath)
	}

	return uncommittedFiles, nil
}

// GetModifiedSourceFiles returns list of modified/added source code files
// in the target repository, excluding project-dir and /tmp/ files.
//
// Algorithm:
//  1. Run: git status --porcelain (from projectDir context)
//  2. Parse output for modified (M) and added (A) files
//  3. Filter out files in project-dir (research code allowed)
//  4. Filter out files in /tmp/ (temporary files allowed)
//  5. Filter by source code extensions (.go, .py, .js, .ts, etc.)
//  6. Return remaining files as violations
//
// Returns empty list if:
//   - No git repo found (not an error, just skip validation)
//   - All changes are in project-dir or /tmp/
//   - All changes are non-code files (.md, .yaml, .sh)
func (g *GitIntegrator) GetModifiedSourceFiles(projectDir string) ([]string, error) {
	// Check if git repo exists
	if !g.IsGitRepo() {
		return []string{}, nil // Not an error - return empty list to skip validation
	}

	// Execute git status --porcelain
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w (output: %s)", err, string(output))
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var violations []string

	for _, line := range lines {
		if len(line) < 3 {
			continue // Skip empty lines
		}

		statusCode := line[0:2]
		filePath := strings.TrimSpace(line[3:])

		// Skip unmodified and ignored files
		// Note: Untracked files (??) are included as potential violations
		if statusCode == "  " || statusCode == "!!" {
			continue
		}

		// Skip files in project-dir (research code allowed)
		if isInProjectDir(filePath, projectDir) {
			continue
		}

		// Skip files in /tmp/ (temporary files allowed)
		if strings.HasPrefix(filePath, "/tmp/") {
			continue
		}

		// Check if file is source code
		if isSourceCodeFile(filePath) {
			violations = append(violations, filePath)
		}
	}

	return violations, nil
}
