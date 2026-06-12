package commands

import (
	"os"
	"path/filepath"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDirectory = ""
			SetProjectDirectory(tt.dir)
			result := GetProjectDirectory()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestGetProjectDirectoryDefault verifies fallback to "." when no STATUS file found
func TestGetProjectDirectoryDefault(t *testing.T) {
	// Run from a temp dir with no WAYFINDER-STATUS.md so auto-detect finds nothing.
	t.Chdir(t.TempDir())

	projectDirectory = ""
	result := GetProjectDirectory()
	if result != "." {
		t.Errorf("expected default %q, got %q", ".", result)
	}
}

// TestGetProjectDirectoryAfterSet verifies retrieval after explicit set
func TestGetProjectDirectoryAfterSet(t *testing.T) {
	testDir := "/test/project/path"
	SetProjectDirectory(testDir)
	result := GetProjectDirectory()
	if result != testDir {
		t.Errorf("expected %q, got %q", testDir, result)
	}
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
	projectDirectory = ""
}

// realPath resolves symlinks so macOS /var → /private/var comparisons work.
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", p, err)
	}
	return r
}

// TestGetProjectDirectoryAutoDetect verifies upward search finds WAYFINDER-STATUS.md
func TestGetProjectDirectoryAutoDetect(t *testing.T) {
	// Create: tmpDir/project/WAYFINDER-STATUS.md
	//         tmpDir/project/subdir/  ← CWD
	projectDir := realPath(t, t.TempDir())
	subDir := filepath.Join(projectDir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(projectDir, statusFilename)
	if err := os.WriteFile(statusPath, []byte("---\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(subDir)

	projectDirectory = ""
	result := GetProjectDirectory()
	if result != projectDir {
		t.Errorf("auto-detect: expected %q, got %q", projectDir, result)
	}
}

// TestGetProjectDirectoryAutoDetect_StatusInCWD verifies detection in CWD itself
func TestGetProjectDirectoryAutoDetect_StatusInCWD(t *testing.T) {
	projectDir := realPath(t, t.TempDir())
	statusPath := filepath.Join(projectDir, statusFilename)
	if err := os.WriteFile(statusPath, []byte("---\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	projectDirectory = ""
	result := GetProjectDirectory()
	if result != projectDir {
		t.Errorf("expected %q, got %q", projectDir, result)
	}
}
