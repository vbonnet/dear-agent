package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateDeliverableExists checks if a deliverable file exists for the given phase
// Pattern: {PHASE}-*.md (e.g., CHARTER-intake.md, RESEARCH-options.md)
// Special case: CHARTER is optional (returns nil if CHARTER was never started)
// Returns ValidationError if no matching file found
func validateDeliverableExists(projectDir, phaseName string) error {
	// CHARTER is optional - if never started, skip validation
	if phaseName == "CHARTER" {
		// Check if CHARTER-*.md exists, if not, assume it was never started
		pattern := filepath.Join(projectDir, "CHARTER-*.md")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("failed to search for deliverable: %w", err)
		}
		if len(matches) == 0 {
			// CHARTER was never started, skip validation
			return nil
		}
	}

	// Search for deliverable matching pattern {PHASE}-*.md
	pattern := filepath.Join(projectDir, fmt.Sprintf("%s-*.md", phaseName))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to search for deliverable: %v", err),
			"Check project directory permissions and try again",
		)
	}

	if len(matches) == 0 {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("no deliverable file found matching pattern %s-*.md", phaseName),
			fmt.Sprintf("Create a deliverable file like %s-{descriptive-name}.md before completing this phase", phaseName),
		)
	}

	// Found at least one deliverable
	return nil
}

// validateDeliverableSize checks if the deliverable file is at least 100 bytes
// Returns ValidationError if file is too small (likely empty or stub)
func validateDeliverableSize(projectDir, phaseName string) error {
	// Find deliverable file
	pattern := filepath.Join(projectDir, fmt.Sprintf("%s-*.md", phaseName))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to search for deliverable: %w", err)
	}

	if len(matches) == 0 {
		// No deliverable found - this should be caught by validateDeliverableExists
		return nil
	}

	// Check size of first matching deliverable
	deliverablePath := matches[0]
	fileInfo, err := os.Stat(deliverablePath)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to stat deliverable file %s: %v", filepath.Base(deliverablePath), err),
			"Ensure the deliverable file exists and is accessible",
		)
	}

	const minSize = 100 // bytes
	if fileInfo.Size() < minSize {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("deliverable file %s is too small (%d bytes, minimum %d bytes)",
				filepath.Base(deliverablePath), fileInfo.Size(), minSize),
			"Add meaningful content to the deliverable before completing this phase",
		)
	}

	return nil
}

// validateBuildImplementation checks that BUILD phase has either:
// - A BUILD-*.md deliverable, OR
// - At least one code file (common extensions: .go, .py, .js, .ts, .java, .c, .cpp, .rs, etc.)
// Returns ValidationError if neither condition is met
func validateBuildImplementation(projectDir string) error {
	// Check for BUILD-*.md deliverable
	pattern := filepath.Join(projectDir, "BUILD-*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to search for BUILD deliverable: %w", err)
	}

	if len(matches) > 0 {
		// Found BUILD markdown deliverable
		return nil
	}

	// No BUILD-*.md found, check for code files
	codeExtensions := []string{
		".go", ".py", ".js", ".ts", ".tsx", ".jsx",
		".java", ".c", ".cpp", ".cc", ".h", ".hpp",
		".rs", ".rb", ".php", ".swift", ".kt", ".cs",
		".sh", ".bash", ".sql", ".r", ".m", ".scala",
	}

	// Search for code files in project directory
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		for _, ext := range codeExtensions {
			if strings.HasSuffix(name, ext) {
				// Found at least one code file
				return nil
			}
		}
	}

	// Neither BUILD-*.md nor code files found
	return NewValidationError(
		"complete BUILD",
		"no implementation artifacts found (expected either BUILD-*.md or code files)",
		"BUILD (Implementation) requires either a BUILD-implementation.md deliverable or actual code files. Add one before completing.",
	)
}
