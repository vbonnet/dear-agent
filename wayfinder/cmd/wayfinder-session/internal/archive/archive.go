// Package archive provides archive-related functionality.
package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// ArchiveManager handles archiving phases before rewind
type ArchiveManager struct {
	projectDir string
}

// New creates a new ArchiveManager for the given project directory
func New(projectDir string) *ArchiveManager {
	return &ArchiveManager{
		projectDir: projectDir,
	}
}

// ArchivePhase creates a snapshot of the current phase state before rewinding
// Archives STATUS and HISTORY files with timestamp
func (a *ArchiveManager) ArchivePhase(phaseName string) error {
	timestamp := time.Now().Format("20060102-150405.000")
	archiveDir := filepath.Join(a.projectDir, ".wayfinder", "archives", phaseName+"-"+timestamp)

	// Create archive directory
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Archive STATUS file
	statusSrc := filepath.Join(a.projectDir, status.StatusFilename)
	statusDst := filepath.Join(archiveDir, status.StatusFilename)
	if err := copyFile(statusSrc, statusDst); err != nil {
		return fmt.Errorf("failed to archive STATUS file: %w", err)
	}

	// Archive HISTORY file if it exists
	historySrc := filepath.Join(a.projectDir, "WAYFINDER-HISTORY.md")
	if _, err := os.Stat(historySrc); err == nil {
		historyDst := filepath.Join(archiveDir, "WAYFINDER-HISTORY.md")
		if err := copyFile(historySrc, historyDst); err != nil {
			return fmt.Errorf("failed to archive HISTORY file: %w", err)
		}
	}

	return nil
}

// ListArchives returns all archived phase snapshots
func (a *ArchiveManager) ListArchives() ([]ArchiveInfo, error) {
	archiveBasePath := filepath.Join(a.projectDir, ".wayfinder", "archives")

	// Check if archives directory exists
	if _, err := os.Stat(archiveBasePath); os.IsNotExist(err) {
		return []ArchiveInfo{}, nil // No archives yet
	}

	entries, err := os.ReadDir(archiveBasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archives directory: %w", err)
	}

	var archives []ArchiveInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't stat
		}

		archives = append(archives, ArchiveInfo{
			Name:      entry.Name(),
			Timestamp: info.ModTime(),
			Path:      filepath.Join(archiveBasePath, entry.Name()),
		})
	}

	return archives, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0o600)
}

// ArchiveInfo contains metadata about an archived phase
type ArchiveInfo struct {
	Name      string
	Timestamp time.Time
	Path      string
}
