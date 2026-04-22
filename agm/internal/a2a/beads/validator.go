package beads

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidationResult holds the outcome of a bead description validation
type ValidationResult struct {
	Valid           bool
	InvalidFileRefs []InvalidFileRef
	Warnings        []string
}

// InvalidFileRef represents a file path referenced in a bead description that doesn't exist
type InvalidFileRef struct {
	Path   string
	Reason string
}

// DescriptionValidator validates bead descriptions for factual accuracy
type DescriptionValidator struct {
	RepoRoot string
}

// NewDescriptionValidator creates a new validator rooted at the given directory
func NewDescriptionValidator(repoRoot string) *DescriptionValidator {
	return &DescriptionValidator{RepoRoot: repoRoot}
}

var fileRefPatterns = []*regexp.Regexp{
	regexp.MustCompile("`([a-zA-Z0-9_./-]+\\.[a-zA-Z0-9]+)`"),
	regexp.MustCompile(`(?:^|\s)((?:[a-zA-Z0-9_-]+/)+[a-zA-Z0-9_.-]+\.[a-zA-Z0-9]+)(?::\d+)?(?:\s|$|[,;)])`),
	regexp.MustCompile(`(?i)(?:file|path|source|target):\s*([a-zA-Z0-9_./-]+\.[a-zA-Z0-9]+)`),
}

var excludedExtensions = map[string]bool{
	".com": true, ".org": true, ".net": true, ".io": true,
	".html": true,
}

// ExtractFileRefs extracts file path references from a bead description
func ExtractFileRefs(description string) []string {
	seen := make(map[string]bool)
	var refs []string
	for _, pattern := range fileRefPatterns {
		matches := pattern.FindAllStringSubmatch(description, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			path := strings.TrimSpace(match[1])
			if path == "" || len(path) < 3 || seen[path] {
				continue
			}
			if strings.Contains(path, "://") {
				continue
			}
			ext := filepath.Ext(path)
			if excludedExtensions[ext] {
				continue
			}
			codeExts := map[string]bool{
				".go": true, ".py": true, ".sh": true, ".js": true, ".ts": true,
				".yaml": true, ".yml": true, ".json": true, ".md": true, ".toml": true,
				".sql": true, ".rs": true, ".rb": true, ".java": true, ".c": true,
				".h": true, ".cpp": true, ".proto": true, ".css": true, ".tsx": true,
				".jsx": true, ".vue": true, ".tf": true, ".mod": true, ".sum": true,
			}
			if !strings.Contains(path, "/") && !codeExts[ext] {
				continue
			}
			seen[path] = true
			refs = append(refs, path)
		}
	}
	return refs
}

// ValidateDescription checks a bead description for factual accuracy
func (v *DescriptionValidator) ValidateDescription(description string) ValidationResult {
	result := ValidationResult{Valid: true}
	refs := ExtractFileRefs(description)
	if len(refs) == 0 {
		return result
	}
	for _, ref := range refs {
		fullPath := filepath.Join(v.RepoRoot, ref)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			result.Valid = false
			result.InvalidFileRefs = append(result.InvalidFileRefs, InvalidFileRef{
				Path:   ref,
				Reason: "file not found in repository",
			})
		}
	}
	if len(result.InvalidFileRefs) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d of %d file references could not be verified",
				len(result.InvalidFileRefs), len(refs)))
	}
	return result
}

// FormatValidationReport returns a human-readable report of validation results
func FormatValidationReport(result ValidationResult) string {
	if result.Valid {
		return "Bead description validation passed"
	}
	var sb strings.Builder
	sb.WriteString("Bead description validation failed\n")
	for _, ref := range result.InvalidFileRefs {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", ref.Path, ref.Reason))
	}
	for _, w := range result.Warnings {
		sb.WriteString(fmt.Sprintf("  Warning: %s\n", w))
	}
	return sb.String()
}
