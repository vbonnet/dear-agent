package w0

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FileMetadata holds frontmatter metadata for W0 charters.
type FileMetadata struct {
	Created string // ISO date (YYYY-MM-DD)
	Status  string // "draft", "approved", "revised"
	Version string // optional
}

// WriteResult holds the result of a write operation.
type WriteResult struct {
	Success bool
	Path    string
	Error   string
}

// SaveCharter saves a W0 charter to the project directory.
func SaveCharter(projectPath, charter string, metadata ...FileMetadata) WriteResult {
	meta := FileMetadata{
		Created: time.Now().Format("2006-01-02"),
		Status:  "approved",
	}
	if len(metadata) > 0 {
		meta = metadata[0]
	}

	if projectPath == "" || strings.TrimSpace(projectPath) == "" {
		return WriteResult{Error: "Project path is required"}
	}

	if charter == "" || strings.TrimSpace(charter) == "" {
		return WriteResult{Error: "Charter content is required"}
	}

	info, err := os.Stat(projectPath)
	if err != nil || !info.IsDir() {
		return WriteResult{Error: fmt.Sprintf("Project directory does not exist: %s", projectPath)}
	}

	content := FormatCharterWithFrontmatter(charter, meta)
	filePath := filepath.Join(projectPath, "W0-project-charter.md")

	// Backup existing file
	if _, err := os.Stat(filePath); err == nil {
		backupPath := filepath.Join(projectPath,
			fmt.Sprintf("W0-project-charter.backup-%d.md", time.Now().UnixMilli()))
		data, _ := os.ReadFile(filePath)
		if data != nil {
			_ = os.WriteFile(backupPath, data, 0o644)
		}
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return WriteResult{Error: err.Error()}
	}

	return WriteResult{Success: true, Path: filePath}
}

// FormatCharterWithFrontmatter formats a charter with YAML frontmatter.
func FormatCharterWithFrontmatter(charter string, metadata FileMetadata) string {
	frontmatter := GenerateFrontmatter(metadata)
	return frontmatter + "\n\n" + strings.TrimSpace(charter) + "\n"
}

// GenerateFrontmatter generates YAML frontmatter.
func GenerateFrontmatter(metadata FileMetadata) string {
	lines := []string{"---"}
	lines = append(lines, fmt.Sprintf("created: %s", metadata.Created))
	lines = append(lines, fmt.Sprintf("status: %s", metadata.Status))
	if metadata.Version != "" {
		lines = append(lines, fmt.Sprintf("version: %s", metadata.Version))
	}
	lines = append(lines, "---")
	return strings.Join(lines, "\n")
}

// ReadCharter reads an existing W0 charter from the project directory.
func ReadCharter(projectPath string) (content string, metadata FileMetadata, err error) {
	filePath := filepath.Join(projectPath, "W0-project-charter.md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", FileMetadata{}, fmt.Errorf("W0 charter file does not exist")
	}

	charter, meta := ParseCharterWithFrontmatter(string(data))
	return charter, meta, nil
}

// ParseCharterWithFrontmatter parses charter content and extracts frontmatter.
func ParseCharterWithFrontmatter(content string) (string, FileMetadata) {
	lines := strings.Split(content, "\n")

	defaultMeta := FileMetadata{
		Created: time.Now().Format("2006-01-02"),
		Status:  "draft",
	}

	if len(lines) == 0 || lines[0] != "---" {
		return content, defaultMeta
	}

	endIndex := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIndex = i
			break
		}
	}

	if endIndex == -1 {
		return content, defaultMeta
	}

	meta := defaultMeta
	kvRegex := regexp.MustCompile(`^(\w+):\s*(.+)$`)
	for _, line := range lines[1:endIndex] {
		matches := kvRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		key, value := matches[1], strings.TrimSpace(matches[2])
		switch key {
		case "created":
			meta.Created = value
		case "status":
			meta.Status = value
		case "version":
			meta.Version = value
		}
	}

	charter := strings.TrimSpace(strings.Join(lines[endIndex+1:], "\n"))
	return charter, meta
}

// CharterExists checks if a W0 charter exists in the project directory.
func CharterExists(projectPath string) bool {
	filePath := filepath.Join(projectPath, "W0-project-charter.md")
	_, err := os.Stat(filePath)
	return err == nil
}

// DeleteCharter deletes the W0 charter from the project directory.
func DeleteCharter(projectPath string) WriteResult {
	filePath := filepath.Join(projectPath, "W0-project-charter.md")
	if _, err := os.Stat(filePath); err != nil {
		return WriteResult{Error: "W0 charter file does not exist"}
	}

	if err := os.Remove(filePath); err != nil {
		return WriteResult{Error: err.Error()}
	}

	return WriteResult{Success: true, Path: filePath}
}
