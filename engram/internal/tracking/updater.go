package tracking

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/pkg/engram"
	"gopkg.in/yaml.v3"
)

// MetadataUpdater handles atomic updates to engram frontmatter
type MetadataUpdater struct {
	parser *engram.Parser
}

// NewMetadataUpdater creates a new metadata updater
func NewMetadataUpdater() *MetadataUpdater {
	return &MetadataUpdater{
		parser: engram.NewParser(),
	}
}

// UpdateMetadata atomically updates an engram's metadata fields.
//
// This function:
//  1. Reads and parses the current engram
//  2. Updates retrieval_count and last_accessed
//  3. Writes atomically using temp file + rename
//
// Parameters:
//   - path: Absolute path to the engram file
//   - record: Access record containing count and timestamp
//
// Returns error if read, parse, or write fails.
// Does not modify file if any step fails (atomicity guarantee).
func (u *MetadataUpdater) UpdateMetadata(path string, record *AccessRecord) error {
	// 1. Read current engram
	eg, err := u.parser.Parse(path)
	if err != nil {
		return fmt.Errorf("failed to parse engram: %w", err)
	}

	// 2. Update metadata fields
	eg.Frontmatter.RetrievalCount += record.Count
	eg.Frontmatter.LastAccessed = record.LastAccess

	// Initialize CreatedAt if not set (legacy engrams)
	if eg.Frontmatter.CreatedAt.IsZero() {
		// Use file mtime as best approximation
		info, err := os.Stat(path)
		if err == nil {
			eg.Frontmatter.CreatedAt = info.ModTime()
		} else {
			// Fallback to current time if stat fails
			eg.Frontmatter.CreatedAt = record.LastAccess
		}
	}

	// Initialize EncodingStrength if not set (should already be set by parser, but double-check)
	if eg.Frontmatter.EncodingStrength == 0.0 {
		eg.Frontmatter.EncodingStrength = 1.0 // Default neutral
	}

	// 3. Serialize to YAML frontmatter + content
	data, err := u.serialize(eg)
	if err != nil {
		return fmt.Errorf("failed to serialize engram: %w", err)
	}

	// 4. Atomic write (temp file + rename)
	tmpPath := path + ".tmp"

	// Get original file permissions
	info, err := os.Stat(path)
	var fileMode os.FileMode = 0644 // Default
	if err == nil {
		fileMode = info.Mode()
	}

	// Write to temp file
	if err := os.WriteFile(tmpPath, data, fileMode); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename (POSIX guarantees atomicity)
	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on failure
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// serialize converts an Engram back to .ai.md format
func (u *MetadataUpdater) serialize(eg *engram.Engram) ([]byte, error) {
	// Serialize frontmatter to YAML
	fmBytes, err := yaml.Marshal(eg.Frontmatter)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Build complete file: ---\n<frontmatter>\n---\n<content>
	var result []byte
	result = append(result, []byte("---\n")...)
	result = append(result, fmBytes...)
	result = append(result, []byte("---\n")...)
	result = append(result, []byte(eg.Content)...)

	return result, nil
}
