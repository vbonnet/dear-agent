package stophook

import (
	"os"
	"path/filepath"
)

// HasWayfinder returns true if the directory contains Wayfinder project markers.
func HasWayfinder(dir string) bool {
	markers := []string{
		"WAYFINDER-STATUS.md",
		".wayfinder",
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

// DetectTestFramework returns the test framework detected in the directory.
// Returns empty string if none detected.
func DetectTestFramework(dir string) string {
	checks := []struct {
		file      string
		framework string
	}{
		{"go.mod", "go"},
		{"package.json", "npm"},
		{"pytest.ini", "pytest"},
		{"setup.py", "pytest"},
		{"pyproject.toml", "pytest"},
		{"Cargo.toml", "cargo"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.framework
		}
	}
	return ""
}

// FileExists checks if a file exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
