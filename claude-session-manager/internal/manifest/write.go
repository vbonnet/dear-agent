package manifest

import (
	"time"

	"gopkg.in/yaml.v3"
)

const (
	manifestPerm = 0600 // rw------- (user only)
	dirPerm      = 0700 // rwx------ (user only)
)

// Write atomically writes a manifest v2 with automatic UpdatedAt timestamp
// If the manifest already exists, a numbered backup is created automatically
func Write(path string, m *Manifest) error {
	// Set UpdatedAt timestamp
	m.UpdatedAt = time.Now()

	// Ensure schema version is set to v2
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}

	// Use helper for backup + validate + marshal + atomic write
	return writeManifestHelper(
		path,
		m.Validate,
		func() ([]byte, error) { return yaml.Marshal(m) },
		manifestPerm,
	)
}

// WriteV3 atomically writes a manifest v3 with automatic UpdatedAt timestamp
// If the manifest already exists, a numbered backup is created automatically
func WriteV3(path string, m *ManifestV3) error {
	// Set UpdatedAt timestamp
	m.UpdatedAt = time.Now()

	// Ensure schema version is set to v3
	if m.SchemaVersion == "" {
		m.SchemaVersion = "3.0"
	}

	// Use helper for backup + validate + marshal + atomic write
	return writeManifestHelper(
		path,
		m.Validate,
		func() ([]byte, error) { return yaml.Marshal(m) },
		manifestPerm,
	)
}

// Note: Lock/Unlock functions removed - use lock.go AcquireLock/ReleaseLock instead
