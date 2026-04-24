package commands

import (
	"testing"
)

// TestSetProjectDirectory verifies SetProjectDirectory stores the directory
func TestSetProjectDirectory(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		expected string
	}{
		{
			name:     "absolute path",
			dir:      "/tmp/test/project",
			expected: "/tmp/test/project",
		},
		{
			name:     "relative path",
			dir:      "./project",
			expected: "./project",
		},
		{
			name:     "current directory",
			dir:      ".",
			expected: ".",
		},
		{
			name:     "empty string",
			dir:      "",
			expected: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset before each test
			projectDirectory = ""

			// Set directory
			SetProjectDirectory(tt.dir)

			// Get directory
			result := GetProjectDirectory()

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestGetProjectDirectoryDefault verifies default behavior when not set
func TestGetProjectDirectoryDefault(t *testing.T) {
	// Reset to empty
	projectDirectory = ""

	result := GetProjectDirectory()
	expected := "."

	if result != expected {
		t.Errorf("expected default %q, got %q", expected, result)
	}
}

// TestGetProjectDirectoryAfterSet verifies retrieval after setting
func TestGetProjectDirectoryAfterSet(t *testing.T) {
	testDir := "/test/project/path"

	SetProjectDirectory(testDir)
	result := GetProjectDirectory()

	if result != testDir {
		t.Errorf("expected %q, got %q", testDir, result)
	}

	// Cleanup
	projectDirectory = ""
}

// TestMultipleSetCalls verifies last set value is returned
func TestMultipleSetCalls(t *testing.T) {
	SetProjectDirectory("/first/path")
	SetProjectDirectory("/second/path")
	SetProjectDirectory("/third/path")

	result := GetProjectDirectory()
	expected := "/third/path"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Cleanup
	projectDirectory = ""
}
