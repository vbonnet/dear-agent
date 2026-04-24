// Package config provides configuration management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetStoragePath resolves the absolute path where AGM data should be stored
// based on the storage configuration mode (dotfile vs centralized)
func GetStoragePath(cfg *Config) (string, error) {
	// Dotfile mode: use legacy dotfile location
	if cfg.Storage.Mode == "" || cfg.Storage.Mode == "dotfile" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(homeDir, ".agm"), nil
	}

	// Centralized mode: resolve workspace and build path
	if cfg.Storage.Mode == "centralized" {
		workspacePath, err := DetectWorkspace(cfg.Storage.Workspace)
		if err != nil {
			return "", fmt.Errorf("failed to detect workspace: %w", err)
		}

		relativePath := cfg.Storage.RelativePath
		if relativePath == "" {
			relativePath = ".agm" // Default
		}

		storagePath := filepath.Join(workspacePath, relativePath)
		return storagePath, nil
	}

	return "", fmt.Errorf("invalid storage mode: %s (must be 'dotfile' or 'centralized')", cfg.Storage.Mode)
}

// DetectWorkspace implements workspace detection with multiple strategies
// Priority order:
// 1. Absolute path provided
// 2. Test mode (ENGRAM_TEST_MODE + ENGRAM_TEST_WORKSPACE)
// 3. Environment variable (ENGRAM_WORKSPACE)
// 4. Auto-detect from current directory
// 5. Search common locations
// 6. Error (interactive prompt not supported in AGM)
func DetectWorkspace(nameOrPath string) (string, error) {
	// Priority 1: Absolute path provided
	if filepath.IsAbs(nameOrPath) {
		if _, err := os.Stat(nameOrPath); err == nil {
			return nameOrPath, nil
		}
		return "", fmt.Errorf("workspace path does not exist: %s", nameOrPath)
	}

	// Priority 2: Test mode
	if os.Getenv("ENGRAM_TEST_MODE") == "1" {
		testWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")
		if testWorkspace != "" {
			return testWorkspace, nil
		}
	}

	// Priority 3: Environment variable override
	if envWorkspace := os.Getenv("ENGRAM_WORKSPACE"); envWorkspace != "" {
		return envWorkspace, nil
	}

	// Priority 4: Auto-detect from current directory
	if workspace := searchUpwardForWorkspace(nameOrPath); workspace != "" {
		return workspace, nil
	}

	// Priority 5: Search common locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	commonLocations := []string{
		filepath.Join(homeDir, "src", "ws", "oss", "repos", nameOrPath),
		filepath.Join(homeDir, "src", "ws", nameOrPath, "repos", nameOrPath),
		filepath.Join(homeDir, "src", nameOrPath),
		filepath.Join(homeDir, nameOrPath),
	}

	for _, loc := range commonLocations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	// Priority 6: Error (interactive prompt not supported)
	return "", fmt.Errorf("workspace '%s' not found (tried common locations, set ENGRAM_WORKSPACE env var or use absolute path)", nameOrPath)
}

// searchUpwardForWorkspace searches parent directories for workspace markers
func searchUpwardForWorkspace(targetName string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := cwd
	for {
		// Check for workspace markers
		if hasWorkspaceMarker(dir, targetName) {
			return dir
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return ""
}

// hasWorkspaceMarker checks if directory has workspace identification markers
func hasWorkspaceMarker(dir, targetName string) bool {
	// Check for .git directory
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Verify it's the right workspace by checking directory name
		if targetName == "" || filepath.Base(dir) == targetName {
			return true
		}
	}

	// Check for WORKSPACE.yaml
	if _, err := os.Stat(filepath.Join(dir, "WORKSPACE.yaml")); err == nil {
		return true
	}

	return false
}

// EnsureSymlinkBootstrap creates a symlink from dotfile location to centralized storage
// This is called when centralized mode is enabled to ensure transparent redirection
func EnsureSymlinkBootstrap(cfg *Config) error {
	// Only create symlink in centralized mode
	if cfg.Storage.Mode != "centralized" {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	dotfilePath := filepath.Join(homeDir, ".agm")
	centralizedPath, err := GetStoragePath(cfg)
	if err != nil {
		return fmt.Errorf("failed to resolve centralized storage path: %w", err)
	}

	// Step 1: Check if dotfile path exists
	info, err := os.Lstat(dotfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist - create symlink directly
			return createSymlink(centralizedPath, dotfilePath)
		}
		return fmt.Errorf("failed to check dotfile path: %w", err)
	}

	// Step 2: Handle existing symlink
	if info.Mode()&os.ModeSymlink != 0 {
		// Already a symlink - verify it points to correct location
		target, err := os.Readlink(dotfilePath)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}

		// Resolve target to absolute path for comparison
		absTarget, err := filepath.Abs(target)
		if err != nil {
			absTarget = target // Use as-is if abs fails
		}

		if absTarget == centralizedPath {
			// Already configured correctly
			return nil
		}

		// Symlink points to wrong location - remove and recreate
		if err := os.Remove(dotfilePath); err != nil {
			return fmt.Errorf("failed to remove old symlink: %w", err)
		}
		return createSymlink(centralizedPath, dotfilePath)
	}

	// Step 3: Handle existing directory - migrate data
	return migrateToSymlink(dotfilePath, centralizedPath)
}

// createSymlink creates a symlink and ensures the target directory exists
func createSymlink(target, link string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Create symlink
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// migrateToSymlink migrates data from dotfile to centralized location and creates symlink
func migrateToSymlink(dotfilePath, centralizedPath string) error {
	// Backup existing dotfile directory
	backupPath := fmt.Sprintf("%s.backup.%s", dotfilePath, fmt.Sprintf("%d", os.Getpid()))

	// Rename dotfile to backup
	if err := os.Rename(dotfilePath, backupPath); err != nil {
		return fmt.Errorf("failed to backup dotfile: %w", err)
	}

	// Ensure centralized directory exists
	if err := os.MkdirAll(centralizedPath, 0755); err != nil {
		// Rollback on failure
		os.Rename(backupPath, dotfilePath)
		return fmt.Errorf("failed to create centralized directory: %w", err)
	}

	// Copy data from backup to centralized location
	if err := copyDir(backupPath, centralizedPath); err != nil {
		// Rollback on failure
		os.RemoveAll(centralizedPath)
		os.Rename(backupPath, dotfilePath)
		return fmt.Errorf("failed to copy data to centralized location: %w", err)
	}

	// Create symlink
	if err := os.Symlink(centralizedPath, dotfilePath); err != nil {
		// Rollback on failure
		os.RemoveAll(centralizedPath)
		os.Rename(backupPath, dotfilePath)
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Success - keep backup for safety
	fmt.Fprintf(os.Stderr, "Migrated AGM data from %s to %s (backup: %s)\n", dotfilePath, centralizedPath, backupPath)
	return nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip if it's the root directory
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		return copyFile(path, dstPath, info.Mode())
	})
}

// copyFile copies a single file
func copyFile(src, dst string, mode os.FileMode) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, mode)
}

// VerifyStorageIntegrity checks that storage is configured correctly
func VerifyStorageIntegrity(cfg *Config) error {
	storagePath, err := GetStoragePath(cfg)
	if err != nil {
		return fmt.Errorf("failed to resolve storage path: %w", err)
	}

	// Check if storage path exists
	info, err := os.Stat(storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("storage path does not exist: %s", storagePath)
		}
		return fmt.Errorf("failed to access storage path: %w", err)
	}

	// Check if it's a directory
	if !info.IsDir() {
		return fmt.Errorf("storage path is not a directory: %s", storagePath)
	}

	// Check if writable
	testFile := filepath.Join(storagePath, ".agm-test-write")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("storage path is not writable: %w", err)
	}
	os.Remove(testFile)

	// If centralized mode, verify symlink
	if cfg.Storage.Mode == "centralized" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		dotfilePath := filepath.Join(homeDir, ".agm")
		linkInfo, err := os.Lstat(dotfilePath)
		if err != nil {
			return fmt.Errorf("symlink check failed: %w", err)
		}

		if linkInfo.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("expected symlink at %s but found regular directory", dotfilePath)
		}

		target, err := os.Readlink(dotfilePath)
		if err != nil {
			return fmt.Errorf("failed to read symlink target: %w", err)
		}

		// Resolve to absolute path
		absTarget, err := filepath.Abs(target)
		if err == nil {
			target = absTarget
		}

		if !strings.HasSuffix(target, storagePath) && target != storagePath {
			return fmt.Errorf("symlink points to wrong location: %s (expected: %s)", target, storagePath)
		}
	}

	return nil
}
