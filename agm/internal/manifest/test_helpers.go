package manifest

import (
	"errors"
	"os"
)

// DEPRECATED: Write/Read/List functions removed in Phase 6 (YAML backend removal)
// These stubs exist only for backward compatibility during test migration
// All tests should be updated to use Dolt adapter directly

// ErrYAMLBackendRemoved is returned by deprecated stubs to flag callers still on the removed YAML backend.
var ErrYAMLBackendRemoved = errors.New("YAML backend removed in Phase 6 - use Dolt adapter directly")

// Write is a deprecated stub (YAML backend removed in Phase 6)
func Write(path string, m *Manifest) error {
	// No-op for tests that haven't been migrated yet
	return nil
}

// Read is a deprecated stub (YAML backend removed in Phase 6)
func Read(path string) (*Manifest, error) {
	return nil, os.ErrNotExist
}

// List is a deprecated stub (YAML backend removed in Phase 6)
func List(sessionsDir string) ([]*Manifest, error) {
	return nil, ErrYAMLBackendRemoved
}
