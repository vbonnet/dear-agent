package configloader

import (
	"fmt"
	"os"
	"path/filepath"
)

// ExpandHome expands ~ to the user's home directory.
//
// Handles edge cases:
//   - "~" alone → home directory
//   - "~/path" → home/path
//   - "/absolute/path" → unchanged
//   - "relative/path" → unchanged
//   - "" → unchanged (empty string)
//   - "~something" → unchanged (not expanded, rare case)
//
// Returns an error only if os.UserHomeDir() fails, which is rare
// (typically only in containerized environments without HOME set).
//
// Example:
//
//	path, err := ExpandHome("~/.config/app")
//	// Returns: "~/.config/app"
//
//	path, err := ExpandHome("/etc/app/config.yaml")
//	// Returns: "/etc/app/config.yaml" (unchanged)
func ExpandHome(path string) (string, error) {
	// Empty path - return as-is
	if len(path) == 0 {
		return path, nil
	}

	// Only expand if starts with ~
	if path[0] != '~' {
		return path, nil
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}

	// Handle "~" alone
	if len(path) == 1 {
		return homeDir, nil
	}

	// Handle "~/..." (most common case)
	if path[1] == '/' || path[1] == filepath.Separator {
		return filepath.Join(homeDir, path[2:]), nil
	}

	// "~something" is not expanded (rare case, not standard behavior)
	// This matches bash behavior where "~user" would expand to user's home,
	// but we only support current user's home (~/)
	return path, nil
}
