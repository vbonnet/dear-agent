package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Red flag patterns for design-only language detection
var redFlagPatterns = []string{
	"would implement",
	"demonstration",
	"blueprint",
	"conceptual",
	"ready for implementation",
	"what would be",
	"example implementation",
	"design phase",
	"placeholder",
	"to be implemented",
}

// scanForRedFlags scans markdown files for design-only language patterns
// Returns ValidationError if red flags found (with file:line context)
// Returns nil if no red flags detected
func scanForRedFlags(projectDir string) error {
	// Scan ARCHITECTURE.md and *.md files in project root
	mdFiles, err := filepath.Glob(filepath.Join(projectDir, "*.md"))
	if err != nil {
		return fmt.Errorf("failed to scan for markdown files: %w", err)
	}

	for _, mdFile := range mdFiles {
		content, err := os.ReadFile(mdFile)
		if err != nil {
			continue // Skip unreadable files
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			lineLower := strings.ToLower(line)
			for _, pattern := range redFlagPatterns {
				if strings.Contains(lineLower, pattern) {
					return NewValidationError(
						"complete BUILD",
						fmt.Sprintf("Design document detected instead of implementation\n\nRed flag found in %s:\n  Line %d: %q\n\nS8 is for IMPLEMENTATION, not design. Common causes:\n- Agent wrote planning docs instead of code\n- Copy-pasted from S6 Design phase\n- Placeholder text not replaced\n\nFix by:\n1. Write actual code (*.go, *.py, *.ts files)\n2. Remove design language from documentation\n3. Update %s to describe what WAS built, not what WILL BE built",
							filepath.Base(mdFile), lineNum+1, strings.TrimSpace(line), filepath.Base(mdFile)),
						"Remove design language and implement actual code",
					)
				}
			}
		}
	}

	return nil
}

// verifyBuildArtifacts checks that all artifacts in BuildArtifacts list exist
// Returns ValidationError if any artifacts missing (with list of missing)
// Returns nil if all artifacts exist or list is empty
func verifyBuildArtifacts(projectDir string, artifacts []string) error {
	if len(artifacts) == 0 {
		return nil // No artifacts specified (not an error)
	}

	var missing []string
	for _, artifact := range artifacts {
		// Validate path (security check)
		if err := validateArtifactPath(projectDir, artifact); err != nil {
			return NewValidationError(
				"complete BUILD",
				fmt.Sprintf("Path traversal detected: %s", artifact),
				"Use relative paths within project directory or safe absolute paths",
			)
		}

		// Check if artifact exists
		exists, err := checkArtifact(projectDir, artifact)
		if err != nil {
			return fmt.Errorf("failed to check artifact %s: %w", artifact, err)
		}

		if !exists {
			missing = append(missing, artifact)
		}
	}

	if len(missing) > 0 {
		return NewValidationError(
			"complete BUILD",
			fmt.Sprintf("Build artifacts missing:\n\nExpected artifacts:\n%s\n\nBuild completed successfully but expected artifacts were not created.\nCheck build_artifacts in ARCHITECTURE.md frontmatter.",
				formatMissingArtifacts(projectDir, artifacts, missing)),
			"Verify build command creates all artifacts or update ARCHITECTURE.md frontmatter",
		)
	}

	return nil
}

// checkArtifact verifies single artifact exists
// Handles both absolute (/tmp/build/app) and relative (bin/app) paths
// Returns (true, nil) if artifact exists
// Returns (false, nil) if artifact missing (validation failure, not error)
// Returns (false, error) if permission denied or other I/O error
func checkArtifact(projectDir, artifact string) (bool, error) {
	var path string
	if filepath.IsAbs(artifact) {
		path = artifact // Absolute: /tmp/build/app
	} else {
		path = filepath.Join(projectDir, artifact) // Relative: bin/app
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // File doesn't exist (not an error)
		}
		return false, err // Permission denied, etc. (error)
	}
	return true, nil // File exists
}

// validateArtifactPath prevents path traversal attacks
// Returns nil if path is safe (within project directory)
// Returns error if path escapes project directory (../../etc/passwd)
func validateArtifactPath(projectDir, artifact string) error {
	// Absolute paths are allowed (user may specify /tmp/build/app)
	if filepath.IsAbs(artifact) {
		return nil
	}

	// Resolve to absolute path
	absPath := filepath.Clean(filepath.Join(projectDir, artifact))

	// Ensure path is within project directory
	cleanProjectDir := filepath.Clean(projectDir)
	if !strings.HasPrefix(absPath, cleanProjectDir) {
		return fmt.Errorf("artifact path traversal detected: %s escapes project directory", artifact)
	}

	return nil
}

// formatMissingArtifacts formats artifact list with exists/missing markers
func formatMissingArtifacts(projectDir string, all []string, missing []string) string {
	missingSet := make(map[string]bool)
	for _, m := range missing {
		missingSet[m] = true
	}

	var lines []string
	for _, artifact := range all {
		if missingSet[artifact] {
			lines = append(lines, fmt.Sprintf("  ❌ %s (missing)", artifact))
		} else {
			lines = append(lines, fmt.Sprintf("  ✅ %s (exists)", artifact))
		}
	}
	return strings.Join(lines, "\n")
}
