package context

import "path/filepath"

// DefaultContextPath returns the path for the "current" session context file.
// This is a convenience wrapper for hooks that always write to the same file.
func DefaultContextPath() string {
	return filepath.Join(DefaultContextDir(), "current.json")
}
