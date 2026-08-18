package validator

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// validateGitCommitStatus checks if deliverable files are committed to git
// This prevents the "fake implementation" bug (oss-55e) where agents claim files
// are "already committed" but they're actually untracked.
//
// For BUILD (Implementation): Check that deliverables have no uncommitted changes
// For RETRO (Retrospective): Check that all phase deliverables are committed
//
// Returns ValidationError if:
// - Files exist but are untracked or modified (documented but not committed)
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

	// Get all uncommitted paths in the project directory.
	uncommittedFiles, err := getUncommittedFilesInProjectDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	// Collect all violations
	violations := checkDeliverableMarkdown(projectDir, phaseName, uncommittedFiles)
	violations = append(violations, checkCodeFiles(projectDir, phaseName, uncommittedFiles)...)

	if len(violations) > 0 {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("deliverable files exist but are not committed to git: %s", strings.Join(violations, ", ")),
			"Commit these files to git before completing this phase. This prevents the 'fake implementation' bug (oss-55e) where agents claim deployment without executing git commands.",
		)
	}

	return nil
}

// checkDeliverableMarkdown checks for uncommitted deliverable markdown files.
// The current phase artifact is committed by CommitPhaseCompletion after all
// validation gates pass, so it must remain reachable here. Artifacts from any
// other phase still prove that an earlier deliverable was never committed.
func checkDeliverableMarkdown(projectDir, currentPhase string, uncommittedFiles []string) []string {
	violations := []string{}
	allPhases := []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"}

	for _, phase := range allPhases {
		if phase == currentPhase {
			continue
		}
		pattern := fmt.Sprintf("%s-*.md", phase)
		matches, err := filepath.Glob(filepath.Join(projectDir, pattern))
		if err != nil {
			continue
		}

		for _, match := range matches {
			fileName := filepath.Base(match)
			if isFileUncommitted(fileName, uncommittedFiles) {
				violations = append(violations, fileName)
			}
		}
	}

	return violations
}

// checkCodeFiles checks for uncommitted code files (BUILD only).
func checkCodeFiles(_ string, phaseName string, uncommittedFiles []string) []string {
	if phaseName != "BUILD" {
		return []string{}
	}

	violations := []string{}
	codeExtensions := []string{
		".go", ".py", ".js", ".ts", ".tsx", ".jsx",
		".java", ".c", ".cpp", ".rs", ".sh", ".bash",
	}

	for _, name := range uncommittedFiles {
		for _, ext := range codeExtensions {
			if strings.HasSuffix(name, ext) {
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

// getUncommittedFilesInProjectDir returns modified, staged, deleted, renamed,
// copied, and untracked paths in the project directory.
func getUncommittedFilesInProjectDir(projectDir string) ([]string, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w (output: %s)", err, string(output))
	}

	var uncommittedFiles []string
	entries := strings.Split(string(output), "\x00")

	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		if len(entry) < 4 {
			continue
		}

		statusCode := entry[0:2]
		filePath := entry[3:]
		if !isWayfinderInternalPath(filePath) {
			uncommittedFiles = append(uncommittedFiles, filePath)
		}

		// Porcelain v1 -z emits a second NUL-delimited source path after a
		// rename or copy. Both source and destination are relevant: a BUILD can
		// rename a source file to a non-source extension and still be dirty.
		if strings.ContainsAny(statusCode, "RC") && index+1 < len(entries) {
			index++
			sourcePath := entries[index]
			if sourcePath != "" && !isWayfinderInternalPath(sourcePath) {
				uncommittedFiles = append(uncommittedFiles, sourcePath)
			}
		}
	}

	return uncommittedFiles, nil
}

func isWayfinderInternalPath(filePath string) bool {
	return strings.HasPrefix(filePath, ".wayfinder/") || strings.Contains(filePath, "/.wayfinder/")
}

// isFileUncommitted checks if filename is in the uncommitted files list.
func isFileUncommitted(fileName string, uncommittedFiles []string) bool {
	for _, uncommitted := range uncommittedFiles {
		// Match exact filename or filename as path component
		if uncommitted == fileName || strings.HasSuffix(uncommitted, "/"+fileName) {
			return true
		}
	}
	return false
}
