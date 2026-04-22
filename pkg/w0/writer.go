package w0

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// CharterFilename is the standard name for W0 charter files.
	CharterFilename = "W0-project-charter.md"

	// BackupTemplate is the filename template for charter backups.
	BackupTemplate = "W0-project-charter.backup-%d.md"

	// DefaultFileMode is the file permission mode for created charter files (rw-r--r--).
	DefaultFileMode = 0o644
)

// Status represents the approval status of a W0 charter.
type Status string

const (
	// StatusDraft indicates the charter is in draft state.
	StatusDraft Status = "draft"
	// StatusApproved indicates the charter has been approved.
	StatusApproved Status = "approved"
	// StatusRevised indicates the charter has been revised.
	StatusRevised Status = "revised"
)

// Metadata represents the YAML frontmatter metadata for a W0 charter.
type Metadata struct {
	Created string  // ISO date (YYYY-MM-DD)
	Status  Status  // Approval status
	Version *string // Optional version identifier
}

// WriteResult represents the outcome of a write operation.
type WriteResult struct {
	Success bool
	Path    string
	Error   error
}

// ReadResult represents the outcome of a read operation.
type ReadResult struct {
	Success  bool
	Content  string
	Metadata *Metadata
	Error    error
}

// SaveCharter saves a W0 charter to the project directory with YAML frontmatter.
//
// The function performs the following:
//   - Validates inputs (projectPath and charter required, non-empty)
//   - Checks that the project directory exists and is writable
//   - Backs up any existing charter file with a timestamp suffix
//   - Writes the charter atomically using a temp file + rename pattern
//   - Applies proper file permissions (0644)
//
// If metadata is nil, defaults are used (today's date, approved status).
//
// Returns WriteResult with success status, file path, or error details.
func SaveCharter(projectPath, charter string, metadata *Metadata) WriteResult {
	// Validate inputs
	if strings.TrimSpace(projectPath) == "" {
		return WriteResult{
			Success: false,
			Error:   errors.New("project path is required"),
		}
	}

	if strings.TrimSpace(charter) == "" {
		return WriteResult{
			Success: false,
			Error:   errors.New("charter content is required"),
		}
	}

	// Check if project directory exists
	info, err := os.Stat(projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return WriteResult{
				Success: false,
				Error:   fmt.Errorf("project directory does not exist: %s", projectPath),
			}
		}
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("stat project directory: %w", err),
		}
	}

	if !info.IsDir() {
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("project path is not a directory: %s", projectPath),
		}
	}

	// Check if directory is writable
	// Try creating a temp file as the most reliable write check
	tempFile := filepath.Join(projectPath, ".w0-write-test")
	if err := os.WriteFile(tempFile, []byte{}, DefaultFileMode); err != nil {
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("project directory is not writable: %s", projectPath),
		}
	}
	_ = os.Remove(tempFile) // Clean up test file, ignore error

	// Use defaults if metadata is nil
	if metadata == nil {
		today := time.Now().Format("2006-01-02")
		metadata = &Metadata{
			Created: today,
			Status:  StatusApproved,
		}
	}

	// Generate file content with frontmatter
	content := FormatCharterWithFrontmatter(charter, metadata)

	// Construct file path
	filePath := filepath.Join(projectPath, CharterFilename)

	// Backup existing file if present
	if _, err := os.Stat(filePath); err == nil {
		backupPath := filepath.Join(projectPath, fmt.Sprintf(BackupTemplate, time.Now().UnixMilli()))
		if err := copyFile(filePath, backupPath); err != nil {
			return WriteResult{
				Success: false,
				Error:   fmt.Errorf("backup existing charter: %w", err),
			}
		}
	}

	// Write file atomically using temp + rename pattern
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content), DefaultFileMode); err != nil {
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("write temp file: %w", err),
		}
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath) // Clean up temp file on error, ignore error
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("rename temp file: %w", err),
		}
	}

	return WriteResult{
		Success: true,
		Path:    filePath,
	}
}

// FormatCharterWithFrontmatter formats a charter with YAML frontmatter.
//
// The output format is:
//
//	---
//	created: YYYY-MM-DD
//	status: approved
//	version: 1.0 (if present)
//	---
//
//	<charter content>
func FormatCharterWithFrontmatter(charter string, metadata *Metadata) string {
	frontmatter := GenerateFrontmatter(metadata)
	trimmedCharter := strings.TrimSpace(charter)
	return fmt.Sprintf("%s\n\n%s\n", frontmatter, trimmedCharter)
}

// GenerateFrontmatter generates YAML frontmatter from metadata.
func GenerateFrontmatter(metadata *Metadata) string {
	var lines []string
	lines = append(lines, "---")
	lines = append(lines, fmt.Sprintf("created: %s", metadata.Created))
	lines = append(lines, fmt.Sprintf("status: %s", metadata.Status))

	if metadata.Version != nil {
		lines = append(lines, fmt.Sprintf("version: %s", *metadata.Version))
	}

	lines = append(lines, "---")
	return strings.Join(lines, "\n")
}

// ReadCharter reads an existing W0 charter from the project directory.
//
// Returns ReadResult with success status, content, metadata, or error.
func ReadCharter(projectPath string) ReadResult {
	filePath := filepath.Join(projectPath, CharterFilename)

	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return ReadResult{
				Success: false,
				Error:   errors.New("W0 charter file does not exist"),
			}
		}
		return ReadResult{
			Success: false,
			Error:   fmt.Errorf("stat charter file: %w", err),
		}
	}

	// #nosec G304 -- projectPath is controlled by caller, filePath is constructed internally
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ReadResult{
			Success: false,
			Error:   fmt.Errorf("read charter file: %w", err),
		}
	}

	charter, metadata := ParseCharterWithFrontmatter(string(data))

	return ReadResult{
		Success:  true,
		Content:  charter,
		Metadata: metadata,
	}
}

// ParseCharterWithFrontmatter parses charter content and extracts frontmatter.
//
// If no valid frontmatter is found, returns the entire content as charter
// with default metadata (today's date, draft status).
func ParseCharterWithFrontmatter(content string) (string, *Metadata) {
	lines := strings.Split(content, "\n")

	// Check if starts with frontmatter
	if len(lines) == 0 || lines[0] != "---" {
		return content, &Metadata{
			Created: time.Now().Format("2006-01-02"),
			Status:  StatusDraft,
		}
	}

	// Find end of frontmatter
	endIndex := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIndex = i
			break
		}
	}

	if endIndex == -1 {
		// Malformed frontmatter
		return content, &Metadata{
			Created: time.Now().Format("2006-01-02"),
			Status:  StatusDraft,
		}
	}

	// Parse frontmatter
	frontmatterLines := lines[1:endIndex]
	metadata := &Metadata{
		Created: time.Now().Format("2006-01-02"),
		Status:  StatusDraft,
	}

	// Regex to match YAML key-value pairs
	re := regexp.MustCompile(`^(\w+):\s*(.+)$`)

	for _, line := range frontmatterLines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			key := matches[1]
			value := strings.TrimSpace(matches[2])

			switch key {
			case "created":
				metadata.Created = value
			case "status":
				metadata.Status = Status(value)
			case "version":
				metadata.Version = &value
			}
		}
	}

	// Extract charter content (everything after frontmatter)
	charter := strings.TrimSpace(strings.Join(lines[endIndex+1:], "\n"))

	return charter, metadata
}

// CharterExists checks if a W0 charter exists in the project directory.
func CharterExists(projectPath string) bool {
	filePath := filepath.Join(projectPath, CharterFilename)
	_, err := os.Stat(filePath)
	return err == nil
}

// DeleteCharter deletes a W0 charter from the project directory.
// Use with caution.
func DeleteCharter(projectPath string) WriteResult {
	filePath := filepath.Join(projectPath, CharterFilename)

	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return WriteResult{
				Success: false,
				Error:   errors.New("W0 charter file does not exist"),
			}
		}
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("stat charter file: %w", err),
		}
	}

	if err := os.Remove(filePath); err != nil {
		return WriteResult{
			Success: false,
			Error:   fmt.Errorf("delete charter file: %w", err),
		}
	}

	return WriteResult{
		Success: true,
		Path:    filePath,
	}
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	// #nosec G304 -- src is validated by caller (charter file path)
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}

	if err := os.WriteFile(dst, data, DefaultFileMode); err != nil {
		return fmt.Errorf("write destination file: %w", err)
	}

	return nil
}
