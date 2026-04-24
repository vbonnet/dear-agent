package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ParseV2 reads and parses a V2 WAYFINDER-STATUS.md file
func ParseV2(filePath string) (*StatusV2, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract YAML frontmatter
	yamlContent, err := extractV2Frontmatter(string(data))
	if err != nil {
		return nil, err
	}

	var status StatusV2
	if err := yaml.Unmarshal([]byte(yamlContent), &status); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &status, nil
}

// ParseV2FromDir reads WAYFINDER-STATUS.md from a directory
func ParseV2FromDir(dir string) (*StatusV2, error) {
	path := filepath.Join(dir, StatusFilename)
	return ParseV2(path)
}

// WriteV2 writes a V2 StatusV2 struct to a WAYFINDER-STATUS.md file
func WriteV2(status *StatusV2, filePath string) error {
	// Marshal to YAML
	yamlData, err := yaml.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Create content with YAML frontmatter
	// Note: V2 files are pure YAML between --- markers, no markdown body
	content := fmt.Sprintf("---\n%s---\n", string(yamlData))

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// WriteV2ToDir writes a V2 StatusV2 struct to WAYFINDER-STATUS.md in a directory
func WriteV2ToDir(status *StatusV2, dir string) error {
	path := filepath.Join(dir, StatusFilename)
	return WriteV2(status, path)
}

// extractV2Frontmatter extracts YAML between --- delimiters
// Similar to V1 but returns error for missing frontmatter since V2 is pure YAML
func extractV2Frontmatter(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return "", fmt.Errorf("invalid V2 format: file too short")
	}

	if lines[0] != "---" {
		return "", fmt.Errorf("invalid V2 format: must start with ---")
	}

	var yamlLines []string
	foundClosing := false
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			foundClosing = true
			break
		}
		yamlLines = append(yamlLines, lines[i])
	}

	if !foundClosing {
		return "", fmt.Errorf("invalid V2 format: missing closing ---")
	}

	if len(yamlLines) == 0 {
		return "", fmt.Errorf("invalid V2 format: empty YAML content")
	}

	return strings.Join(yamlLines, "\n"), nil
}

// DetectSchemaVersion reads a file and detects whether it's V1 or V2
func DetectSchemaVersion(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	yamlContent, err := extractV2Frontmatter(string(data))
	if err != nil {
		// Try V1 format
		yamlContent, err = extractFrontmatter(string(data))
		if err != nil {
			return "", fmt.Errorf("failed to parse frontmatter: %w", err)
		}
	}

	// Parse just the schema_version field
	var metadata struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal([]byte(yamlContent), &metadata); err != nil {
		return "", fmt.Errorf("failed to parse schema_version: %w", err)
	}

	if metadata.SchemaVersion == "" {
		return "1.0", nil // Default to V1
	}

	return metadata.SchemaVersion, nil
}

// NewStatusV2 creates a new V2 status with default values
func NewStatusV2(projectName, projectType, riskLevel string) *StatusV2 {
	now := time.Now()
	return &StatusV2{
		SchemaVersion:   SchemaVersionV2,
		ProjectName:     projectName,
		ProjectType:     projectType,
		RiskLevel:       riskLevel,
		CurrentWaypoint: WaypointV2Charter, // Start at CHARTER
		Status:          StatusV2Planning,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{},
		Roadmap: &Roadmap{
			Phases: []RoadmapPhase{},
		},
	}
}
