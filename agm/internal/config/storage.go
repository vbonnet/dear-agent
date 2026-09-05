// Package config provides configuration management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetStoragePath returns the physical storage root retained by Load.
func GetStoragePath(cfg *Config) (string, error) {
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		return "", err
	}
	storage, err := authority.Storage()
	if err != nil {
		return "", err
	}
	return storage.Path()
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return detectWorkspaceAt(nameOrPath, homeDir)
}

func detectWorkspaceAt(nameOrPath, homeDir string) (string, error) {
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
	return ensureSymlinkBootstrap(cfg, false)
}

// EnsureSymlinkBootstrapQuiet performs the same required storage bootstrap
// without writing its best-effort migration notice to stderr. Machine-facing
// command surfaces use this when stderr is reserved for one structured record.
func EnsureSymlinkBootstrapQuiet(cfg *Config) error {
	return ensureSymlinkBootstrap(cfg, true)
}

func ensureSymlinkBootstrap(cfg *Config, quiet bool) error {
	dotfilePath, centralizedPath, centralized, err := centralizedBootstrapPaths(cfg)
	if err != nil {
		return err
	}
	if !centralized {
		return nil
	}

	info, err := os.Lstat(dotfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return createSymlink(centralizedPath, dotfilePath)
		}
		return fmt.Errorf("failed to check dotfile path: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return ensureCentralizedSymlink(dotfilePath, centralizedPath)
	}
	return migrateToSymlink(dotfilePath, centralizedPath, quiet)
}

func centralizedBootstrapPaths(cfg *Config) (dotfilePath, centralizedPath string, centralized bool, err error) {
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to resolve runtime authority: %w", err)
	}
	if !authority.centralized {
		return "", "", false, nil
	}
	home, err := authority.Home()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to resolve retained home: %w", err)
	}
	homeDir, err := home.Path()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to resolve retained home path: %w", err)
	}
	storage, err := authority.Storage()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to resolve retained storage: %w", err)
	}
	centralizedPath, err = storage.Path()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to resolve retained storage path: %w", err)
	}
	return filepath.Join(homeDir, ".agm"), centralizedPath, true, nil
}

func ensureCentralizedSymlink(dotfilePath, centralizedPath string) error {
	target, err := os.Readlink(dotfilePath)
	if err != nil {
		return fmt.Errorf("failed to read symlink: %w", err)
	}
	absoluteTarget := target
	if !filepath.IsAbs(absoluteTarget) {
		absoluteTarget = filepath.Join(filepath.Dir(dotfilePath), absoluteTarget)
	}
	resolvedTarget, resolveErr := resolvePhysicalDirectory(filepath.Clean(absoluteTarget))
	if resolveErr == nil && resolvedTarget == centralizedPath {
		if err := os.MkdirAll(centralizedPath, 0o700); err != nil {
			return fmt.Errorf("failed to ensure centralized target: %w", err)
		}
		return nil
	}
	if err := os.Remove(dotfilePath); err != nil {
		return fmt.Errorf("failed to remove old symlink: %w", err)
	}
	return createSymlink(centralizedPath, dotfilePath)
}

// createSymlink creates a symlink and ensures the target directory exists
func createSymlink(target, link string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Create symlink
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// migrateToSymlink migrates data from dotfile to centralized location and creates symlink
func migrateToSymlink(dotfilePath, centralizedPath string, quiet bool) error {
	// Backup existing dotfile directory
	backupPath := fmt.Sprintf("%s.backup.%s", dotfilePath, fmt.Sprintf("%d", os.Getpid()))

	// Rename dotfile to backup
	if err := os.Rename(dotfilePath, backupPath); err != nil {
		return fmt.Errorf("failed to backup dotfile: %w", err)
	}

	// Ensure centralized directory exists
	if err := os.MkdirAll(centralizedPath, 0o700); err != nil {
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
	if !quiet {
		fmt.Fprintf(os.Stderr, "Migrated AGM data from %s to %s (backup: %s)\n", dotfilePath, centralizedPath, backupPath)
	}
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
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		return fmt.Errorf("failed to resolve runtime authority: %w", err)
	}
	storage, err := authority.Storage()
	if err != nil {
		return fmt.Errorf("failed to resolve retained storage: %w", err)
	}
	storagePath, err := storage.Path()
	if err != nil {
		return fmt.Errorf("failed to resolve retained storage path: %w", err)
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
	testFile, err := os.CreateTemp(storagePath, ".agm-test-write-*")
	if err != nil {
		return fmt.Errorf("storage path is not writable: %w", err)
	}
	testPath := testFile.Name()
	if err := testFile.Close(); err != nil {
		_ = os.Remove(testPath)
		return fmt.Errorf("close storage write probe: %w", err)
	}
	if err := os.Remove(testPath); err != nil {
		return fmt.Errorf("remove storage write probe: %w", err)
	}

	// If centralized mode, verify symlink
	if authority.centralized {
		home, err := authority.Home()
		if err != nil {
			return fmt.Errorf("failed to resolve retained home: %w", err)
		}
		homeDir, err := home.Path()
		if err != nil {
			return fmt.Errorf("failed to resolve retained home path: %w", err)
		}

		dotfilePath := filepath.Join(homeDir, ".agm")
		linkInfo, err := os.Lstat(dotfilePath)
		if err != nil {
			return fmt.Errorf("symlink check failed: %w", err)
		}

		if linkInfo.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("expected symlink at %s but found regular directory", dotfilePath)
		}

		resolvedTarget, err := filepath.EvalSymlinks(dotfilePath)
		if err != nil {
			return fmt.Errorf("failed to resolve symlink target: %w", err)
		}
		if filepath.Clean(resolvedTarget) != storagePath {
			return fmt.Errorf("symlink points to wrong location: %s (expected: %s)", resolvedTarget, storagePath)
		}
	}

	return nil
}
