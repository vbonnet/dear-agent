package commands

import (
	"os"
	"path/filepath"
)

const statusFilename = "WAYFINDER-STATUS.md"

var projectDirectory string

// SetProjectDirectory stores the project directory for use by all commands
func SetProjectDirectory(dir string) {
	projectDirectory = dir
}

// GetProjectDirectory returns the project directory.
// If not explicitly set, searches upward from the current working directory
// for a WAYFINDER-STATUS.md file (mirrors how git finds .git).
// Falls back to "." if no status file is found.
func GetProjectDirectory() string {
	if projectDirectory != "" {
		return projectDirectory
	}
	if dir, ok := findStatusFileUpward(); ok {
		return dir
	}
	return "."
}

// findStatusFileUpward walks parent directories from CWD looking for WAYFINDER-STATUS.md.
func findStatusFileUpward() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, statusFilename)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return "", false
}
