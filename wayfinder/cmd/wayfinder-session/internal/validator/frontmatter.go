package validator

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeliverableFrontmatter represents the YAML frontmatter in a deliverable markdown file
type DeliverableFrontmatter struct {
	Phase              string `yaml:"phase"`
	PhaseName          string `yaml:"phase_name"`
	WayfinderSessionID string `yaml:"wayfinder_session_id"`
	CreatedAt          string `yaml:"created_at"`
	PhaseEngramHash    string `yaml:"phase_engram_hash"`
	PhaseEngramPath    string `yaml:"phase_engram_path"`
}

// extractFrontmatter extracts and parses YAML frontmatter from a deliverable file
// Returns DeliverableFrontmatter if valid frontmatter exists
// Returns error if:
// - File cannot be read
// - No frontmatter delimiters found
// - YAML is malformed
// - Missing required fields
func extractFrontmatter(filePath string) (*DeliverableFrontmatter, error) {
	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Check for frontmatter delimiters
	contentStr := string(content)
	if !strings.HasPrefix(contentStr, "---\n") {
		return nil, fmt.Errorf("file does not start with YAML frontmatter delimiter (---)")
	}

	// Find closing delimiter
	lines := strings.Split(contentStr, "\n")
	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closingIdx = i
			break
		}
	}

	if closingIdx == -1 {
		return nil, fmt.Errorf("no closing frontmatter delimiter (---) found")
	}

	// Extract YAML content (between delimiters)
	yamlContent := strings.Join(lines[1:closingIdx], "\n")

	// Parse YAML
	var fm DeliverableFrontmatter
	decoder := yaml.NewDecoder(bytes.NewBufferString(yamlContent))
	decoder.KnownFields(true) // Strict mode: error on unknown fields
	if err := decoder.Decode(&fm); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Validate required fields
	var missing []string
	if fm.Phase == "" {
		missing = append(missing, "phase")
	}
	if fm.PhaseName == "" {
		missing = append(missing, "phase_name")
	}
	if fm.WayfinderSessionID == "" {
		missing = append(missing, "wayfinder_session_id")
	}
	if fm.CreatedAt == "" {
		missing = append(missing, "created_at")
	}
	if fm.PhaseEngramHash == "" {
		missing = append(missing, "phase_engram_hash")
	}
	if fm.PhaseEngramPath == "" {
		missing = append(missing, "phase_engram_path")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required frontmatter fields: %s", strings.Join(missing, ", "))
	}

	return &fm, nil
}

// ArchitectureFrontmatter represents YAML frontmatter from ARCHITECTURE.md
type ArchitectureFrontmatter struct {
	BuildCommand   string   `yaml:"build_command,omitempty"`
	BuildArtifacts []string `yaml:"build_artifacts,omitempty"`
	TestCommand    string   `yaml:"test_command,omitempty"`
}

// TestPlanFrontmatter represents YAML frontmatter from TEST_PLAN.md
type TestPlanFrontmatter struct {
	CoverageThreshold int    `yaml:"coverage_threshold,omitempty"`
	TestCommand       string `yaml:"test_command,omitempty"`
	SkipCoverageCheck bool   `yaml:"skip_coverage_check,omitempty"`
}

// extractFrontmatterContent extracts YAML frontmatter content from markdown
// Returns empty string if no frontmatter found (not an error)
// Returns error if frontmatter is malformed (unclosed)
func extractFrontmatterContent(fileContent string) (string, error) {
	lines := strings.Split(fileContent, "\n")

	// Find first ---
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			start = i
			break
		}
	}
	if start == -1 {
		return "", nil // No frontmatter found (not an error)
	}

	// Find second ---
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", fmt.Errorf("frontmatter not closed (missing second ---)")
	}

	// Extract YAML between delimiters
	frontmatterLines := lines[start+1 : end]
	return strings.Join(frontmatterLines, "\n"), nil
}

// parseArchitectureFrontmatter reads ARCHITECTURE.md and parses frontmatter
// Returns nil, nil if file doesn't exist (optional frontmatter)
// Returns empty struct, nil if frontmatter is empty (optional fields)
// Returns nil, error if YAML is malformed
func parseArchitectureFrontmatter(projectDir string) (*ArchitectureFrontmatter, error) {
	archPath := fmt.Sprintf("%s/ARCHITECTURE.md", projectDir)

	// Read file
	content, err := os.ReadFile(archPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File missing (not an error)
		}
		return nil, fmt.Errorf("failed to read ARCHITECTURE.md: %w", err)
	}

	// Extract frontmatter
	yamlContent, err := extractFrontmatterContent(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to extract frontmatter from ARCHITECTURE.md: %w", err)
	}

	// If no frontmatter, return empty struct
	if yamlContent == "" {
		return &ArchitectureFrontmatter{}, nil
	}

	// Parse YAML
	var fm ArchitectureFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse ARCHITECTURE.md frontmatter: %w", err)
	}

	return &fm, nil
}

// parseTestPlanFrontmatter reads TEST_PLAN.md and parses frontmatter
// Returns nil, nil if file doesn't exist (optional frontmatter)
// Returns empty struct, nil if frontmatter is empty (optional fields)
// Returns nil, error if YAML is malformed
func parseTestPlanFrontmatter(projectDir string) (*TestPlanFrontmatter, error) {
	testPlanPath := fmt.Sprintf("%s/TEST_PLAN.md", projectDir)

	// Read file
	content, err := os.ReadFile(testPlanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File missing (not an error)
		}
		return nil, fmt.Errorf("failed to read TEST_PLAN.md: %w", err)
	}

	// Extract frontmatter
	yamlContent, err := extractFrontmatterContent(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to extract frontmatter from TEST_PLAN.md: %w", err)
	}

	// If no frontmatter, return empty struct
	if yamlContent == "" {
		return &TestPlanFrontmatter{}, nil
	}

	// Parse YAML
	var fm TestPlanFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse TEST_PLAN.md frontmatter: %w", err)
	}

	return &fm, nil
}
