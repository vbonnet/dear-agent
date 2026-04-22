package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/state"
)

// StatusFileWriter writes session state to JSON files
type StatusFileWriter struct {
	baseDir string
}

// NewStatusFileWriter creates a new status file writer
func NewStatusFileWriter(baseDir string) (*StatusFileWriter, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create status directory: %w", err)
	}

	return &StatusFileWriter{
		baseDir: baseDir,
	}, nil
}

// WriteStatus writes session status to JSON file
func (w *StatusFileWriter) WriteStatus(sessionName string, result state.DetectionResult) error {
	status := StatusResponse{
		SessionName: sessionName,
		State:       result.State,
		Timestamp:   result.Timestamp,
		Evidence:    result.Evidence,
		Confidence:  result.Confidence,
		LastUpdated: time.Now(),
	}

	filePath := w.getStatusFilePath(sessionName)

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	// Write atomically via temp file + rename
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) // Cleanup on error
		return fmt.Errorf("failed to rename status file: %w", err)
	}

	return nil
}

// ReadStatus reads session status from JSON file
func (w *StatusFileWriter) ReadStatus(sessionName string) (*StatusResponse, error) {
	filePath := w.getStatusFilePath(sessionName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("status file not found for session: %s", sessionName)
		}
		return nil, fmt.Errorf("failed to read status file: %w", err)
	}

	var status StatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status file: %w", err)
	}

	return &status, nil
}

// DeleteStatus removes status file for a session
func (w *StatusFileWriter) DeleteStatus(sessionName string) error {
	filePath := w.getStatusFilePath(sessionName)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete status file: %w", err)
	}

	return nil
}

// ListSessions returns all sessions with status files
func (w *StatusFileWriter) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(w.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read status directory: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			// Remove .json extension to get session name
			sessionName := name[:len(name)-5]
			sessions = append(sessions, sessionName)
		}
	}

	return sessions, nil
}

// getStatusFilePath returns the file path for a session's status
func (w *StatusFileWriter) getStatusFilePath(sessionName string) string {
	return filepath.Join(w.baseDir, fmt.Sprintf("%s.json", sessionName))
}

// GetBaseDir returns the base directory for status files
func (w *StatusFileWriter) GetBaseDir() string {
	return w.baseDir
}
