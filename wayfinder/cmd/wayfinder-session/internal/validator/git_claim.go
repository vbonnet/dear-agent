package validator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// validateGitCommitStatus checks if deliverable files are committed to git
// This prevents the "fake implementation" bug (oss-55e) where agents claim files
// are "already committed" but they're actually untracked.
//
// For BUILD (Implementation): Check that deliverables are not untracked
// For RETRO (Retrospective): Check that all phase deliverables are committed
//
// Returns ValidationError if:
// - Files exist but are untracked (documented but not committed)
// - Agent claims deployment but files not in git history
func validateGitCommitStatus(projectDir, phaseName string) error {
	// Only validate for phases where git commits are expected
	if !shouldValidateGitCommit(phaseName) {
		return nil
	}

	// Check if we're in a git repository
	if !isGitRepo(projectDir) {
		return nil
	}

	// Get list of untracked files in project directory
	untrackedFiles, err := getUntrackedFilesInProjectDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	// Collect all violations
	violations := checkDeliverableMarkdown(projectDir, untrackedFiles)
	violations = append(violations, checkCodeFiles(projectDir, phaseName, untrackedFiles)...)

	if len(violations) > 0 {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("deliverable files exist but are not committed to git: %s", strings.Join(violations, ", ")),
			"Commit these files to git before completing this phase. This prevents the 'fake implementation' bug (oss-55e) where agents claim deployment without executing git commands.",
		)
	}

	return nil
}

// checkDeliverableMarkdown checks for untracked deliverable markdown files
func checkDeliverableMarkdown(projectDir string, untrackedFiles []string) []string {
	violations := []string{}
	allPhases := []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"}

	for _, phase := range allPhases {
		pattern := fmt.Sprintf("%s-*.md", phase)
		matches, err := filepath.Glob(filepath.Join(projectDir, pattern))
		if err != nil {
			continue
		}

		for _, match := range matches {
			fileName := filepath.Base(match)
			if isFileUntracked(fileName, untrackedFiles) {
				violations = append(violations, fileName)
			}
		}
	}

	return violations
}

// checkCodeFiles checks for untracked code files (BUILD only)
func checkCodeFiles(projectDir, phaseName string, untrackedFiles []string) []string {
	if phaseName != "BUILD" {
		return []string{}
	}

	violations := []string{}
	codeExtensions := []string{
		".go", ".py", ".js", ".ts", ".tsx", ".jsx",
		".java", ".c", ".cpp", ".rs", ".sh", ".bash",
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return violations
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		for _, ext := range codeExtensions {
			if strings.HasSuffix(name, ext) && isFileUntracked(name, untrackedFiles) {
				violations = append(violations, name)
				break // Avoid duplicate entries if multiple extensions match
			}
		}
	}

	return violations
}

// shouldValidateGitCommit returns true if phase requires git commit validation
func shouldValidateGitCommit(phaseName string) bool {
	// Validate for implementation and deployment phases
	return phaseName == "BUILD" || phaseName == "RETRO"
}

// isGitRepo checks if directory is in a git repository
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	err := cmd.Run()
	return err == nil
}

// getUntrackedFilesInProjectDir returns list of untracked files in project directory
func getUntrackedFilesInProjectDir(projectDir string) ([]string, error) {
	cmd := exec.Command("git", "status", "--porcelain", ".")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w (output: %s)", err, string(output))
	}

	var untrackedFiles []string
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		statusCode := line[0:2]
		filePath := strings.TrimSpace(line[3:])

		// Check if file is untracked (??)
		if statusCode == "??" {
			// Skip .wayfinder/ internal directory
			if !strings.HasPrefix(filePath, ".wayfinder/") && !strings.Contains(filePath, "/.wayfinder/") {
				untrackedFiles = append(untrackedFiles, filePath)
			}
		}
	}

	return untrackedFiles, nil
}

// isFileUntracked checks if filename is in the untracked files list
func isFileUntracked(fileName string, untrackedFiles []string) bool {
	for _, untracked := range untrackedFiles {
		// Match exact filename or filename as path component
		if untracked == fileName || strings.HasSuffix(untracked, "/"+fileName) {
			return true
		}
	}
	return false
}
