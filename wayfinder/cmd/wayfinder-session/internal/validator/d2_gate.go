package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxFileSizeBytes = 1048576 // 1MB
	minWordCount     = 200
)

// validateResearchContent checks RESEARCH-existing-solutions.md for required content before allowing DESIGN start.
// Returns ValidationError if RESEARCH is missing, incomplete, or invalid.
// Returns nil if RESEARCH is valid.
func validateResearchContent(projectDir string) error {
	// Build RESEARCH file path
	researchPath := filepath.Join(projectDir, "RESEARCH-existing-solutions.md")

	// Check file size before reading (security: prevent OOM)
	if err := validateFileSize(researchPath); err != nil {
		return err
	}

	// Read RESEARCH file
	data, err := os.ReadFile(researchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"start DESIGN",
				"RESEARCH-existing-solutions.md does not exist",
				"Complete RESEARCH phase with overlap analysis before DESIGN",
			)
		}
		return NewValidationError(
			"start DESIGN",
			fmt.Sprintf("failed to read RESEARCH file: %v", err),
			"Check file permissions and try again",
		)
	}

	content := string(data)

	// Extract and validate overlap percentage
	overlap, err := extractOverlapPercentage(content)
	if err != nil {
		return NewValidationError(
			"start DESIGN",
			"RESEARCH missing overlap assessment",
			"Add 'Overlap: X%' field to RESEARCH-existing-solutions.md (even if 0% for greenfield)",
		)
	}

	// If overlap < 100%, require search methodology
	if overlap < 100 && !hasSearchMethodology(content) {
		return NewValidationError(
			"start DESIGN",
			"RESEARCH missing search methodology (required for overlap < 100%)",
			"Add 'Search methodology' section documenting how search was conducted",
		)
	}

	// Check minimum word count
	if err := validateWordCount(content); err != nil {
		return err
	}

	return nil
}

// extractOverlapPercentage parses "Overlap: X%" field from RESEARCH content.
// Returns the percentage as integer, or error if not found/malformed.
// Handles markdown formatting like **Overlap:** or plain Overlap:
func extractOverlapPercentage(content string) (int, error) {
	// Pattern: optional markdown stars, "Overlap:", whitespace, digits, "%"
	re := regexp.MustCompile(`\*?\*?Overlap:\*?\*?\s*(\d+)%`)
	matches := re.FindStringSubmatch(content)

	if len(matches) < 2 {
		return 0, fmt.Errorf("overlap percentage not found (expected 'Overlap: X%%' format)")
	}

	percentage, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid overlap percentage: %w", err)
	}

	return percentage, nil
}

// hasSearchMethodology checks if RESEARCH content contains a search methodology section.
// Accepts both "Search methodology" and "Search Methodology" (case variations).
func hasSearchMethodology(content string) bool {
	// Check both header formats and inline mentions
	return strings.Contains(content, "Search methodology") ||
		strings.Contains(content, "Search Methodology") ||
		strings.Contains(content, "## Search methodology") ||
		strings.Contains(content, "## Search Methodology")
}

// validateWordCount checks if RESEARCH content meets minimum word count requirement.
// Returns ValidationError if too short, nil otherwise.
func validateWordCount(content string) error {
	// Count words using Fields (splits on whitespace)
	words := strings.Fields(content)
	wordCount := len(words)

	if wordCount < minWordCount {
		return NewValidationError(
			"start DESIGN",
			fmt.Sprintf("RESEARCH file too short (%d words < %d minimum)", wordCount, minWordCount),
			"Expand RESEARCH analysis with search details, findings, and reuse opportunities",
		)
	}

	return nil
}

// validateFileSize checks if RESEARCH file size is within acceptable limits.
// Returns ValidationError if file is too large or missing, nil otherwise.
func validateFileSize(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"start DESIGN",
				"RESEARCH-existing-solutions.md does not exist",
				"Complete RESEARCH phase with overlap analysis before DESIGN",
			)
		}
		return NewValidationError(
			"start DESIGN",
			fmt.Sprintf("failed to check RESEARCH file: %v", err),
			"Check file permissions and try again",
		)
	}

	if info.Size() > maxFileSizeBytes {
		sizeMB := float64(info.Size()) / 1048576.0
		return NewValidationError(
			"start DESIGN",
			fmt.Sprintf("RESEARCH file too large (%.1fMB > 1MB limit)", sizeMB),
			"Reduce RESEARCH file size by removing large code examples or unnecessary content",
		)
	}

	return nil
}
