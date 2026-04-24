package configloader

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolvePath resolves a path relative to a base directory with tilde expansion.
//
// Resolution rules:
//   - Absolute paths (/foo/bar) → returned as-is
//   - Tilde paths (~/foo/bar) → expanded to user's home directory
//   - Relative paths (foo/bar) → resolved relative to baseDir
//   - Empty baseDir → uses current working directory
//
// Example:
//
//	// Resolve config path relative to workspace
//	path, err := ResolvePath("config/app.yaml", "/tmp/workspace")
//	// Returns: "/tmp/workspace/config/app.yaml"
//
//	// Resolve tilde path
//	path, err := ResolvePath("~/.config/app.yaml", "")
//	// Returns: "$HOME/.config/app.yaml"
//
//	// Absolute path unchanged
//	path, err := ResolvePath("/etc/app.yaml", "/tmp/workspace")
//	// Returns: "/etc/app.yaml"
func ResolvePath(path string, baseDir string) (string, error) {
	// Empty path
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}

	// Absolute path - return as-is
	if filepath.IsAbs(path) {
		return path, nil
	}

	// Tilde path - expand home directory
	if len(path) > 0 && path[0] == '~' {
		return ExpandHome(path)
	}

	// Relative path - resolve against baseDir
	if baseDir == "" {
		// Use current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		baseDir = cwd
	}

	// Expand baseDir if it contains tilde
	expandedBase, err := ExpandHome(baseDir)
	if err != nil {
		return "", fmt.Errorf("expand base directory: %w", err)
	}

	// Join and clean the path
	resolved := filepath.Join(expandedBase, path)
	return filepath.Clean(resolved), nil
}

// ResolvePathWithDefaults is like ResolvePath but returns path unchanged on error.
// Useful when you want best-effort path resolution without failing.
//
// Example:
//
//	path := ResolvePathWithDefaults("config.yaml", workspace)
//	// Always returns a valid path (even if baseDir is invalid)
func ResolvePathWithDefaults(path string, baseDir string) string {
	resolved, err := ResolvePath(path, baseDir)
	if err != nil {
		return path // Return original path on error
	}
	return resolved
}

// FindFile searches for a file in multiple directories, returning the first match.
//
// Searches in order:
//  1. Exact path (if absolute or exists relative to cwd)
//  2. Each directory in searchPaths
//
// All paths support tilde expansion.
//
// Example:
//
//	// Search for config in multiple locations
//	searchPaths := []string{
//	    "~/.config/app",
//	    "/etc/app",
//	    "./config",
//	}
//	path, err := FindFile("app.yaml", searchPaths)
//	// Returns first existing: ~/.config/app/app.yaml or /etc/app/app.yaml or ./config/app.yaml
func FindFile(filename string, searchPaths []string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename is empty")
	}

	// Try exact path first
	expandedFilename, err := ExpandHome(filename)
	if err == nil {
		if _, err := os.Stat(expandedFilename); err == nil {
			return expandedFilename, nil
		}
	}

	// Search in each directory
	for _, dir := range searchPaths {
		expandedDir, err := ExpandHome(dir)
		if err != nil {
			continue // Skip invalid paths
		}

		candidate := filepath.Join(expandedDir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("file %q not found in search paths", filename)
}
