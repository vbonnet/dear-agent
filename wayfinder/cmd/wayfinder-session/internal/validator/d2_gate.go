package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

const (
	gateDeploymentDate = "2026-01-20T00:00:00Z"
	maxFileSizeBytes   = 1048576 // 1MB
	minWordCount       = 200
)

// validateD2Content checks D2-existing-solutions.md for required content before allowing D3 start.
// Returns ValidationError if D2 is missing, incomplete, or invalid.
// Returns nil if D2 is valid or project is legacy (created before gate deployment).
func validateD2Content(projectDir string, st status.StatusInterface) error {
	// Check if legacy project (grandfather clause)
	if isLegacyProject(st) {
		fmt.Fprintf(os.Stderr, "⚠️  Legacy project detected (created before %s) - D2 validation skipped\n", gateDeploymentDate)
		return nil
	}

	// Build D2 file path
	d2Path := filepath.Join(projectDir, "D2-existing-solutions.md")

	// Check file size before reading (security: prevent OOM)
	if err := validateFileSize(d2Path); err != nil {
		return err
	}

	// Read D2 file
	data, err := os.ReadFile(d2Path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"start D3",
				"D2-existing-solutions.md does not exist",
				"Complete D2 phase with overlap analysis before D3",
			)
		}
		return NewValidationError(
			"start D3",
			fmt.Sprintf("failed to read D2 file: %v", err),
			"Check file permissions and try again",
		)
	}

	content := string(data)

	// Extract and validate overlap percentage
	overlap, err := extractOverlapPercentage(content)
	if err != nil {
		return NewValidationError(
			"start D3",
			"D2 missing overlap assessment",
			"Add 'Overlap: X%' field to D2-existing-solutions.md (even if 0% for greenfield)",
		)
	}

	// If overlap < 100%, require search methodology
	if overlap < 100 && !hasSearchMethodology(content) {
		return NewValidationError(
			"start D3",
			"D2 missing search methodology (required for overlap < 100%)",
			"Add 'Search methodology' section documenting how search was conducted",
		)
	}

	// Check minimum word count
	if err := validateWordCount(content); err != nil {
		return err
	}

	return nil
}

// isLegacyProject checks if project was started before D2 gate deployment date.
// Returns true for legacy projects (skip D2 validation), false otherwise.
func isLegacyProject(st status.StatusInterface) bool {
	deploymentDate, err := time.Parse(time.RFC3339, gateDeploymentDate)
	if err != nil {
		// Should never happen with hardcoded constant, but be safe
		return false
	}

	// GetStartedAt works for both V1 (StartedAt) and V2 (CreatedAt)
	return st.GetStartedAt().Before(deploymentDate)
}

// extractOverlapPercentage parses "Overlap: X%" field from D2 content.
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

// hasSearchMethodology checks if D2 content contains a search methodology section.
// Accepts both "Search methodology" and "Search Methodology" (case variations).
func hasSearchMethodology(content string) bool {
	// Check both header formats and inline mentions
	return strings.Contains(content, "Search methodology") ||
		strings.Contains(content, "Search Methodology") ||
		strings.Contains(content, "## Search methodology") ||
		strings.Contains(content, "## Search Methodology")
}

// validateWordCount checks if D2 content meets minimum word count requirement.
// Returns ValidationError if too short, nil otherwise.
func validateWordCount(content string) error {
	// Count words using Fields (splits on whitespace)
	words := strings.Fields(content)
	wordCount := len(words)

	if wordCount < minWordCount {
		return NewValidationError(
			"start D3",
			fmt.Sprintf("D2 file too short (%d words < %d minimum)", wordCount, minWordCount),
			"Expand D2 analysis with search details, findings, and reuse opportunities",
		)
	}

	return nil
}

// validateFileSize checks if D2 file size is within acceptable limits.
// Returns ValidationError if file is too large or missing, nil otherwise.
func validateFileSize(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"start D3",
				"D2-existing-solutions.md does not exist",
				"Complete D2 phase with overlap analysis before D3",
			)
		}
		return NewValidationError(
			"start D3",
			fmt.Sprintf("failed to check D2 file: %v", err),
			"Check file permissions and try again",
		)
	}

	if info.Size() > maxFileSizeBytes {
		sizeMB := float64(info.Size()) / 1048576.0
		return NewValidationError(
			"start D3",
			fmt.Sprintf("D2 file too large (%.1fMB > 1MB limit)", sizeMB),
			"Reduce D2 file size by removing large code examples or unnecessary content",
		)
	}

	return nil
}
