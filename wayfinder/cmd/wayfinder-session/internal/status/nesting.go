// Package nesting provides directory hierarchy detection and validation
// for Wayfinder v2's unlimited tasks/ nesting feature.
//
// This package enables parent projects to decompose into child sub-projects
// with independent wayfinder workflows. Key features:
//
//   - Detect child projects in tasks/ subdirectories
//   - Validate completion gates (parents require all children complete)
//   - Support unlimited nesting depth (with safety limits)
//
// Architecture:
//
//	parent/
//	├── WAYFINDER-STATUS.md
//	└── tasks/
//	    ├── child-a/
//	    │   ├── WAYFINDER-STATUS.md
//	    │   └── tasks/
//	    │       └── grandchild/
//	    └── child-b/
//
// See WAYFINDER-SPEC.md Layer 1.5 for full specification.
package status

import (
	"os"
	"path/filepath"
)

const (
	// TasksDirectoryName is the standard subdirectory name for child projects
	TasksDirectoryName = "tasks"

	// MaxNestingDepth is the maximum allowed nesting depth to prevent
	// resource exhaustion and infinite loops
	MaxNestingDepth = 10
)

// HasChildren returns true if directory contains a tasks/ subdirectory
// with at least one child project directory (containing WAYFINDER-STATUS.md).
//
// This function performs a shallow check and does not validate child project
// completeness. Use CheckChildrenComplete for validation logic.
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - true if valid child projects exist, false otherwise
//
// Example:
//
//	if HasChildren("~/wf/parent") {
//	    fmt.Println("Parent has child projects")
//	}
func HasChildren(dir string) bool {
	// Sanitize path
	cleanDir := filepath.Clean(dir)

	tasksDir := filepath.Join(cleanDir, TasksDirectoryName)

	// Check if tasks/ directory exists
	info, err := os.Stat(tasksDir)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		return false
	}

	// Check if tasks/ contains any valid child projects
	children, err := ListChildren(cleanDir)
	if err != nil {
		return false
	}

	return len(children) > 0
}

// ListChildren returns list of child project names in tasks/ subdirectory.
// Only includes directories with WAYFINDER-STATUS.md.
// Symlinks are ignored for security (prevent directory traversal attacks).
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - Slice of child project directory names (not full paths)
//   - Error if tasks/ directory cannot be read
//
// Example:
//
//	children, err := ListChildren("~/wf/parent")
//	// returns: ["child-a", "child-b"]
func ListChildren(dir string) ([]string, error) {
	// Sanitize path
	cleanDir := filepath.Clean(dir)

	tasksDir := filepath.Join(cleanDir, TasksDirectoryName)

	// Check if tasks/ exists
	if _, err := os.Stat(tasksDir); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil // No tasks/ directory = no children (not an error)
		}
		return nil, err // Other errors (permission, etc.) are real errors
	}

	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, err
	}

	var children []string
	for _, entry := range entries {
		// Skip non-directories
		if !entry.IsDir() {
			continue
		}

		childPath := filepath.Join(tasksDir, entry.Name())

		// Security: Reject symlinks to prevent directory traversal
		// Check via Lstat (doesn't follow symlinks)
		info, err := os.Lstat(childPath)
		if err != nil {
			continue // Skip entries we can't stat
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Symlink detected - skip for security
			continue
		}

		// Check if directory contains WAYFINDER-STATUS.md
		statusPath := filepath.Join(childPath, StatusFilename)
		if fileExists(statusPath) {
			children = append(children, entry.Name())
		}
	}

	return children, nil
}

// HasParent returns true if this directory is nested inside another project's tasks/.
//
// Detection logic:
//   - Parent directory must be named "tasks"
//   - Grandparent directory must contain WAYFINDER-STATUS.md
//
// Parameters:
//   - dir: Absolute path to check
//
// Returns:
//   - true if directory is a child project, false otherwise
//
// Example:
//
//	HasParent("~/wf/parent/tasks/child")  // true
//	HasParent("~/wf/parent")              // false
func HasParent(dir string) bool {
	// Sanitize and get absolute path
	cleanDir := filepath.Clean(dir)
	absDir, err := filepath.Abs(cleanDir)
	if err != nil {
		return false
	}

	parent := filepath.Dir(absDir)
	parentBase := filepath.Base(parent)

	// Parent directory must be named "tasks"
	if parentBase != TasksDirectoryName {
		return false
	}

	// Grandparent must have WAYFINDER-STATUS.md
	grandparent := filepath.Dir(parent)
	statusPath := filepath.Join(grandparent, StatusFilename)

	return fileExists(statusPath)
}

// GetParentPath returns the parent project directory, or empty string if no parent.
//
// Returns the grandparent directory if:
//   - Current directory is inside tasks/
//   - Grandparent contains WAYFINDER-STATUS.md
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - Absolute path to parent project, or "" if not a child project
//
// Example:
//
//	GetParentPath("~/wf/parent/tasks/child")
//	// returns: "~/wf/parent"
func GetParentPath(dir string) string {
	// Sanitize and get absolute path
	cleanDir := filepath.Clean(dir)
	absDir, err := filepath.Abs(cleanDir)
	if err != nil {
		return ""
	}

	parent := filepath.Dir(absDir)
	parentBase := filepath.Base(parent)

	// Parent directory must be named "tasks"
	if parentBase != TasksDirectoryName {
		return ""
	}

	// Grandparent must have WAYFINDER-STATUS.md
	grandparent := filepath.Dir(parent)
	statusPath := filepath.Join(grandparent, StatusFilename)

	if fileExists(statusPath) {
		return grandparent
	}

	return ""
}

// IsChildProject returns true if directory path matches pattern: .../tasks/{name}
//
// This is a lightweight check based on path structure only.
// Use HasParent for validation that includes checking for parent's STATUS file.
//
// Parameters:
//   - dir: Path to check (can be relative or absolute)
//
// Returns:
//   - true if path contains tasks/ as parent directory
//
// Example:
//
//	IsChildProject("~/wf/parent/tasks/child")  // true
//	IsChildProject("../tasks/myproject")                // true
//	IsChildProject("~/wf/standalone")          // false
func IsChildProject(dir string) bool {
	cleanDir := filepath.Clean(dir)
	parent := filepath.Dir(cleanDir)
	parentBase := filepath.Base(parent)

	return parentBase == TasksDirectoryName
}

// GetNestingLevel returns the nesting depth of the project.
// Returns 0 for top-level projects, 1 for direct children, 2 for grandchildren, etc.
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - Nesting level (0 = top-level, 1 = child, 2 = grandchild, ...)
//
// Example:
//
//	GetNestingLevel("~/wf/parent")                          // 0
//	GetNestingLevel("~/wf/parent/tasks/child")              // 1
//	GetNestingLevel("~/wf/parent/tasks/child/tasks/grand")  // 2
func GetNestingLevel(dir string) int {
	level := 0
	current := dir

	// Walk up the directory tree counting tasks/ directories
	for {
		parent := GetParentPath(current)
		if parent == "" {
			break // No more parents
		}

		level++
		current = parent

		// Safety: prevent infinite loops
		if level > MaxNestingDepth {
			break
		}
	}

	return level
}

// ValidateNestingDepth checks if directory exceeds maximum nesting depth.
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - Error if nesting depth exceeds MaxNestingDepth, nil otherwise
func ValidateNestingDepth(dir string) error {
	level := GetNestingLevel(dir)
	if level > MaxNestingDepth {
		return ErrMaxDepthExceeded
	}
	return nil
}
