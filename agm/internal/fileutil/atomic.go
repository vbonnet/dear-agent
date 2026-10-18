// Package fileutil provides fileutil functionality.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// AtomicWrite writes data to a file atomically using temp file + rename
// This ensures that the file is never in a partially written state
// POSIX guarantees that rename is atomic
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWrite(path, data, perm, SyncDir)
}

func atomicWrite(path string, data []byte, perm os.FileMode, syncDir func(string) error) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := mkdirAllDurable(dir, 0700, syncDir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temp file in same directory (required for atomic rename)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Cleanup temp file on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	// Persist the final permission metadata together with the contents. Chmod
	// after Sync would leave the returned mode outside the durable file state.
	if err := tmpFile.Chmod(perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Sync contents and final metadata before publishing the file name.
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename (POSIX guarantees atomicity)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("failed to sync parent directory: %w", err)
	}

	// Success - clear defer cleanup
	tmpFile = nil

	return nil
}

// MkdirAllDurable creates every missing directory component and persists each
// new directory entry in its parent before returning. Syncing only the created
// directory is insufficient: after power loss, its parent can otherwise forget
// the name even though files inside the directory were synced successfully.
func MkdirAllDurable(path string, perm os.FileMode) error {
	return mkdirAllDurable(path, perm, SyncDir)
}

func mkdirAllDurable(path string, perm os.FileMode, syncDir func(string) error) error {
	clean := filepath.Clean(path)
	var missing []string
	for current := clean; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("path component %s is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect directory %s: %w", current, err)
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("no existing parent for directory %s", clean)
		}
	}

	if err := os.MkdirAll(clean, perm); err != nil {
		return err
	}
	// Persist from the highest newly-created component downward. This orders
	// each child only after the name that reaches its parent is durable.
	for _, created := range slices.Backward(missing) {
		if err := syncDir(filepath.Dir(created)); err != nil {
			return fmt.Errorf("sync parent of created directory %s: %w", created, err)
		}
	}
	return nil
}

// SyncDir persists directory-entry changes such as file creation, rename, and
// removal. Syncing a file alone does not make the name that reaches it durable
// across power loss.
func SyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close synced directory: %w", err)
	}
	return nil
}
