package status

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/fileutil"
	"gopkg.in/yaml.v3"
)

// ParseV2 reads and parses a V2 WAYFINDER-STATUS.md file
func ParseV2(filePath string) (*StatusV2, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return ParseV2Content(data)
}

// ParseV2Content parses and fully validates canonical V2 status bytes.
func ParseV2Content(data []byte) (*StatusV2, error) {
	// Extract YAML frontmatter
	yamlContent, err := extractV2Frontmatter(string(data))
	if err != nil {
		return nil, err
	}
	var metadata struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal([]byte(yamlContent), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse schema_version: %w", err)
	}
	if metadata.SchemaVersion == "" {
		return nil, fmt.Errorf("schema_version is required; expected %s", SchemaVersion)
	}
	if metadata.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %q; expected %s", metadata.SchemaVersion, SchemaVersion)
	}
	if err := validateV2TimestampScalars(yamlContent); err != nil {
		return nil, err
	}

	var status StatusV2
	decoder := yaml.NewDecoder(strings.NewReader(yamlContent))
	decoder.KnownFields(true)
	if err := decoder.Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	if err := ValidateV2(&status); err != nil {
		return nil, fmt.Errorf("invalid Wayfinder V2 status: %w", err)
	}

	return &status, nil
}

var v2TimestampFields = map[string]bool{
	"created_at":      true,
	"updated_at":      true,
	"completion_date": true,
	"started_at":      true,
	"completed_at":    true,
	"verified_at":     true,
}

var canonicalRFC3339Pattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

// validateV2TimestampScalars rejects YAML's permissive date-only timestamp
// decoding before it can be normalized into time.Time. Canonical status
// documents use RFC3339 consistently across Go and TypeScript consumers.
func validateV2TimestampScalars(yamlContent string) error {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &document); err != nil {
		return fmt.Errorf("failed to inspect timestamps: %w", err)
	}
	return validateV2TimestampNode(&document, "")
}

func validateV2TimestampNode(node *yaml.Node, path string) error {
	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for i, child := range node.Content {
			childPath := path
			if node.Kind == yaml.SequenceNode {
				childPath = fmt.Sprintf("%s[%d]", path, i)
			}
			if err := validateV2TimestampNode(child, childPath); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		return validateV2TimestampMapping(node, path)
	case yaml.AliasNode:
		if node.Alias != nil {
			return validateV2TimestampNode(node.Alias, path)
		}
	case yaml.ScalarNode:
		// Scalars are validated by their containing mapping when relevant.
	}
	return nil
}

func validateV2TimestampMapping(node *yaml.Node, path string) error {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		childPath := key.Value
		if path != "" {
			childPath = path + "." + key.Value
		}
		if v2TimestampFields[key.Value] && value.Tag != "!!null" {
			if value.Kind != yaml.ScalarNode {
				return fmt.Errorf("invalid Wayfinder V2 status: %s must be an RFC3339 timestamp", childPath)
			}
			if !canonicalRFC3339Pattern.MatchString(value.Value) {
				return fmt.Errorf("invalid Wayfinder V2 status: %s must be an RFC3339 timestamp", childPath)
			}
			if _, err := time.Parse(time.RFC3339, value.Value); err != nil {
				return fmt.Errorf("invalid Wayfinder V2 status: %s must be an RFC3339 timestamp", childPath)
			}
		}
		if err := validateV2TimestampNode(value, childPath); err != nil {
			return err
		}
	}
	return nil
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

	if err := fileutil.AtomicWrite(filePath, []byte(content), 0o600); err != nil {
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
// The canonical status is pure YAML between frontmatter delimiters.
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
	closingLine := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			foundClosing = true
			closingLine = i
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
	for _, line := range lines[closingLine+1:] {
		if strings.TrimSpace(line) != "" {
			return "", fmt.Errorf("invalid V2 format: content after closing --- is not allowed")
		}
	}

	return strings.Join(yamlLines, "\n"), nil
}

// DetectSchemaVersion returns the canonical schema version or fails closed.
func DetectSchemaVersion(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	yamlContent, err := extractV2Frontmatter(string(data))
	if err != nil {
		return "", fmt.Errorf("failed to parse canonical frontmatter: %w", err)
	}

	// Parse just the schema_version field
	var metadata struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal([]byte(yamlContent), &metadata); err != nil {
		return "", fmt.Errorf("failed to parse schema_version: %w", err)
	}

	if metadata.SchemaVersion == "" {
		return "", fmt.Errorf("schema_version is required; expected %s", SchemaVersion)
	}
	if metadata.SchemaVersion != SchemaVersion {
		return "", fmt.Errorf("unsupported schema_version %q; expected %s", metadata.SchemaVersion, SchemaVersion)
	}

	return SchemaVersion, nil
}

// DetectSchemaVersionFromDir validates the status schema in dir.
func DetectSchemaVersionFromDir(dir string) (string, error) {
	return DetectSchemaVersion(filepath.Join(dir, StatusFilename))
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
