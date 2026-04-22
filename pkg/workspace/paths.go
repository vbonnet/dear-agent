package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands ~ to user's home directory.
func ExpandHome(path string) string {
	if path == "~" {
		return os.Getenv("HOME")
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

// NormalizePath converts path to absolute, normalized form.
// Resolves symlinks to ensure consistent comparison (e.g., macOS /var → /private/var).
func NormalizePath(path string) (string, error) {
	// Expand home directory
	path = ExpandHome(path)

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Clean the absolute path first
	absPath = filepath.Clean(absPath)

	// Attempt to resolve symlinks for consistent comparison
	// (e.g., macOS /var is a symlink to /private/var)
	// Try resolving the full path first
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		return filepath.Clean(resolved), nil
	}

	// If full path resolution fails, try resolving parent directories
	// This handles cases where the final component doesn't exist yet
	// but parent directories may have symlinks
	parent := filepath.Dir(absPath)
	base := filepath.Base(absPath)

	if parent != absPath && parent != "." && parent != "/" {
		if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Clean(filepath.Join(resolvedParent, base)), nil
		}
	}

	// Fall back to cleaned absolute path if symlink resolution fails entirely
	return absPath, nil
}

// IsSubpath checks if child is within parent directory.
func IsSubpath(parent, child string) bool {
	// Normalize both paths first
	parentNorm, err := NormalizePath(parent)
	if err != nil {
		return false
	}
	childNorm, err := NormalizePath(child)
	if err != nil {
		return false
	}

	// Check if child starts with parent
	// Add trailing slash to avoid false matches (e.g., /foo matching /foobar)
	parentWithSep := parentNorm
	if !strings.HasSuffix(parentWithSep, string(filepath.Separator)) {
		parentWithSep += string(filepath.Separator)
	}

	// Exact match or child is subdirectory
	return childNorm == parentNorm || strings.HasPrefix(childNorm, parentWithSep)
}

// ValidateAbsolutePath ensures path is absolute and valid.
func ValidateAbsolutePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Expand and normalize
	normalized, err := NormalizePath(path)
	if err != nil {
		return err
	}

	// Check if absolute
	if !filepath.IsAbs(normalized) {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	return nil
}
