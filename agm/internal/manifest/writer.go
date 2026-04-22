package manifest

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/backup"
	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
)

// writeManifestHelper consolidates the backup + validate + marshal + atomic write pattern
// This helper eliminates duplication between Write() and WriteV3()
//
// Pattern:
// 1. Create numbered backup if file exists (using backup.CreateBackup)
// 2. Validate manifest using provided validator
// 3. Marshal data using provided marshaler
// 4. Atomic write using fileutil.AtomicWrite
//
// Parameters:
//   - path: destination file path
//   - validateFn: function that validates the manifest
//   - marshalFn: function that marshals the manifest to bytes
//   - perm: file permissions for the written file
//
// Returns:
//   - error: nil on success, wrapped error on failure
func writeManifestHelper(
	path string,
	validateFn func() error,
	marshalFn func() ([]byte, error),
	perm os.FileMode,
) error {
	// Create backup if file exists
	if _, err := os.Stat(path); err == nil {
		// File exists, create backup before overwriting
		if _, err := backup.CreateBackup(path); err != nil {
			return fmt.Errorf("failed to create backup before write: %w", err)
		}
	}

	// Validate before writing
	if err := validateFn(); err != nil {
		return err
	}

	// Marshal to bytes
	data, err := marshalFn()
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Atomic write using fileutil
	if err := fileutil.AtomicWrite(path, data, perm); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}
