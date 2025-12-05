package manifest

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

const (
	manifestPerm = 0600 // rw------- (user only)
	dirPerm      = 0700 // rwx------ (user only)
)

// Write atomically writes a manifest with file locking
func Write(path string, m *Manifest) error {
	// Validate before writing
	if err := Validate(m); err != nil {
		return err
	}

	// Ensure schema version is set
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}

	// Marshal to YAML
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Acquire lock
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, manifestPerm)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	defer lockFile.Close()

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)

	// Write to temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, manifestPerm); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync to disk
	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to open temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	f.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Lock acquires an exclusive lock on manifest file
func Lock(path string) (*os.File, error) {
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, manifestPerm)
	if err != nil {
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return lockFile, nil
}

// Unlock releases lock
func Unlock(lockFile *os.File) error {
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	return lockFile.Close()
}
