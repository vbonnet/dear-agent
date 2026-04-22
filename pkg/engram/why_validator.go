package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WhyFile represents a .why.md rationale file
type WhyFile struct {
	RationaleFor string `yaml:"rationale_for"`
	DecisionDate string `yaml:"decision_date"`
	DecidedBy    string `yaml:"decided_by"`
	ReviewCycle  string `yaml:"review_cycle"`
	Status       string `yaml:"status"`
	SupersededBy string `yaml:"superseded_by"`
	Content      string // Full markdown content
}

// ValidateWhyFile checks if .ai.md has valid .why.md companion
func ValidateWhyFile(aiMdPath string) error {
	// Check .why.md exists
	whyPath := strings.TrimSuffix(aiMdPath, ".ai.md") + ".why.md"
	if _, err := os.Stat(whyPath); os.IsNotExist(err) {
		return fmt.Errorf("missing .why.md companion for %s", aiMdPath)
	}

	// Parse .why.md
	why, err := ParseWhyFile(whyPath)
	if err != nil {
		return fmt.Errorf("invalid .why.md: %w", err)
	}

	// Validate required fields
	if err := validateRequiredFields(why); err != nil {
		return err
	}

	// Validate field values
	if err := validateFieldValues(why); err != nil {
		return err
	}

	// Validate cross-references
	basename := filepath.Base(strings.TrimSuffix(aiMdPath, ".ai.md"))
	if why.RationaleFor != basename {
		return fmt.Errorf("rationale_for mismatch: expected %q, got %q", basename, why.RationaleFor)
	}

	return nil
}

// ParseWhyFile parses a .why.md file
func ParseWhyFile(path string) (*WhyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)

	// Extract frontmatter
	frontmatter, _, err := extractFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract frontmatter: %w", err)
	}

	// Parse YAML
	var why WhyFile
	if err := yaml.Unmarshal([]byte(frontmatter), &why); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	why.Content = content
	return &why, nil
}

// validateRequiredFields checks all required fields present
func validateRequiredFields(why *WhyFile) error {
	if why.RationaleFor == "" {
		return fmt.Errorf("missing required field: rationale_for")
	}
	if why.DecisionDate == "" {
		return fmt.Errorf("missing required field: decision_date")
	}
	if why.DecidedBy == "" {
		return fmt.Errorf("missing required field: decided_by")
	}
	if why.ReviewCycle == "" {
		return fmt.Errorf("missing required field: review_cycle")
	}
	if why.Status == "" {
		return fmt.Errorf("missing required field: status")
	}
	return nil
}

// validateFieldValues checks field value constraints
func validateFieldValues(why *WhyFile) error {
	// Validate status enum
	validStatuses := []string{"active", "deprecated", "superseded"}
	if !containsString(validStatuses, why.Status) {
		return fmt.Errorf("invalid status: %q (must be active|deprecated|superseded)", why.Status)
	}

	// Validate review_cycle enum
	validCycles := []string{"quarterly", "annually", "as-needed"}
	if !containsString(validCycles, why.ReviewCycle) {
		return fmt.Errorf("invalid review_cycle: %q (must be quarterly|annually|as-needed)", why.ReviewCycle)
	}

	// Validate superseded_by if status is superseded
	if why.Status == "superseded" && why.SupersededBy == "" {
		return fmt.Errorf("status=superseded requires superseded_by field")
	}

	// Validate decision_date format (ISO 8601)
	if _, err := time.Parse("2006-01-02", why.DecisionDate); err != nil {
		return fmt.Errorf("invalid decision_date format: %q (expected YYYY-MM-DD)", why.DecisionDate)
	}

	return nil
}

// containsString checks if slice containsString string
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// extractFrontmatter extracts YAML frontmatter from markdown
func extractFrontmatter(content string) (frontmatter, body string, err error) {
	lines := strings.Split(content, "\n")

	if len(lines) < 2 || lines[0] != "---" {
		return "", "", fmt.Errorf("no frontmatter found")
	}

	// Find closing ---
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return "", "", fmt.Errorf("unclosed frontmatter")
	}

	frontmatter = strings.Join(lines[1:endIdx], "\n")
	body = strings.Join(lines[endIdx+1:], "\n")

	return frontmatter, body, nil
}

// RequireWhyFile enforces .why.md presence (used in CI)
func RequireWhyFile(engramPath string) error {
	return ValidateWhyFile(engramPath)
}
