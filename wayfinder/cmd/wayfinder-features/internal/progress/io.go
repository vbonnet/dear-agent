package progress

import (
	"encoding/json"
	"fmt"

	"os"
	"path/filepath"
	"time"
)

// ReadProgress reads progress.json from the specified path
func ReadProgress(path string) (*Progress, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("progress file not found at %s\nRun 'wayfinder-features init' first", path)
		}
		return nil, fmt.Errorf("failed to read progress file: %w", err)
	}

	var progress Progress
	if err := json.Unmarshal(data, &progress); err != nil {
		return nil, fmt.Errorf("failed to parse progress file (corrupted JSON): %w", err)
	}

	// Validate schema version
	if progress.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported schema version %s (expected %s)", progress.SchemaVersion, SchemaVersion)
	}

	return &progress, nil
}

// WriteProgress writes progress.json atomically with backup
func WriteProgress(path string, progress *Progress) error {
	// Update last_updated timestamp
	progress.LastUpdated = time.Now()

	// Marshal to JSON with pretty printing
	data, err := json.MarshalIndent(progress, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal progress: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create backup if file exists
	backupPath := path + ".backup"
	if _, err := os.Stat(path); err == nil {
		if err := copyFile(path, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write to temporary file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	// Delete backup on success
	os.Remove(backupPath)

	return nil
}

// FindFeature searches for a feature by ID
func FindFeature(progress *Progress, featureID string) (*Feature, error) {
	for i := range progress.Features {
		if progress.Features[i].ID == featureID {
			return &progress.Features[i], nil
		}
	}
	return nil, fmt.Errorf("feature '%s' not found in S7 plan", featureID)
}

// UpdateFeature updates a feature in the progress
func UpdateFeature(progress *Progress, featureID string, updater func(*Feature)) error {
	for i := range progress.Features {
		if progress.Features[i].ID == featureID {
			updater(&progress.Features[i])
			return nil
		}
	}
	return fmt.Errorf("feature '%s' not found", featureID)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// FindProgressFile searches up the directory tree for progress.json
func FindProgressFile() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Search up the directory tree
	dir := cwd
	for {
		path := filepath.Join(dir, DefaultProgressFile)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("progress.json not found (searched from %s upwards)\nRun 'wayfinder-features init' to create it", cwd)
}
