package manifest

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/backup"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/fileutil"
	"gopkg.in/yaml.v3"
)

const (
	manifestPerm = 0600 // rw------- (user only)
	dirPerm      = 0700 // rwx------ (user only)
)

// Write atomically writes a manifest v2 with automatic UpdatedAt timestamp
// If the manifest already exists, a numbered backup is created automatically
func Write(path string, m *Manifest) error {
	// Create backup if file exists
	if _, err := os.Stat(path); err == nil {
		// File exists, create backup before overwriting
		if _, err := backup.CreateBackup(path); err != nil {
			return fmt.Errorf("failed to create backup before write: %w", err)
		}
	}

	// Set UpdatedAt timestamp
	m.UpdatedAt = time.Now()

	// Ensure schema version is set to v2
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}

	// Validate before writing (v2 validation)
	if err := m.Validate(); err != nil {
		return err
	}

	// Marshal to YAML
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Atomic write using fileutil
	if err := fileutil.AtomicWrite(path, data, manifestPerm); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// WriteV3 atomically writes a manifest v3 with automatic UpdatedAt timestamp
// If the manifest already exists, a numbered backup is created automatically
func WriteV3(path string, m *ManifestV3) error {
	// Create backup if file exists
	if _, err := os.Stat(path); err == nil {
		// File exists, create backup before overwriting
		if _, err := backup.CreateBackup(path); err != nil {
			return fmt.Errorf("failed to create backup before write: %w", err)
		}
	}

	// Set UpdatedAt timestamp
	m.UpdatedAt = time.Now()

	// Ensure schema version is set to v3
	if m.SchemaVersion == "" {
		m.SchemaVersion = "3.0"
	}

	// Validate before writing (v3 validation)
	if err := m.Validate(); err != nil {
		return err
	}

	// Marshal to YAML
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Atomic write using fileutil
	if err := fileutil.AtomicWrite(path, data, manifestPerm); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// Note: Lock/Unlock functions removed - use lock.go AcquireLock/ReleaseLock instead
