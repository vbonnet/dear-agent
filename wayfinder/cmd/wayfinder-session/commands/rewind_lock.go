package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var errRewindTransitionInProgress = errors.New("another rewind transition is already in progress")

const rewindLockFilename = "rewind.lock"

type rewindTransitionLock interface {
	Close() error
}

func rewindLockFilePath(projectDir string) (string, error) {
	absoluteProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolve rewind project directory: %w", err)
	}
	projectDir = absoluteProjectDir
	// #nosec G703 -- projectDir is the explicit Wayfinder project target; Stat validates it before the owned path is joined.
	info, err := os.Stat(projectDir)
	if err != nil {
		return "", fmt.Errorf("inspect rewind project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("rewind project path is not a directory: %s", projectDir)
	}

	wayfinderDir := filepath.Join(projectDir, ".wayfinder")
	if err := ensureRewindDirectory(wayfinderDir, "metadata"); err != nil {
		return "", err
	}
	if err := validateRewindMetadataDirectory(wayfinderDir); err != nil {
		return "", err
	}
	lockDir := filepath.Join(wayfinderDir, "locks")
	if err := ensureRewindDirectory(lockDir, "lock"); err != nil {
		return "", err
	}
	if err := validatePrivateRewindLockDirectories(wayfinderDir, lockDir); err != nil {
		return "", err
	}
	return filepath.Join(lockDir, rewindLockFilename), nil
}

func ensureRewindDirectory(path, description string) error {
	// #nosec G703 -- path is the explicit project target joined only with fixed internal metadata components.
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create rewind %s directory: %w", description, err)
	}
	// #nosec G703 -- inspect the exact directory just created or found; symbolic links are rejected.
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect rewind %s directory: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("rewind %s path is not an owned directory: %s", description, path)
	}
	return nil
}
