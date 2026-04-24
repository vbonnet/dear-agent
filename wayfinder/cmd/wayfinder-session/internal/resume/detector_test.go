package resume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyState_Empty(t *testing.T) {
	files := []string{}
	result := classifyState(files)

	if result.State != StateEmpty {
		t.Errorf("Expected StateEmpty, got %s", result.State)
	}
	if len(result.VisibleFiles) != 0 {
		t.Errorf("Expected 0 visible files, got %d", len(result.VisibleFiles))
	}
}

func TestClassifyState_W0Only(t *testing.T) {
	tests := []struct {
		name  string
		files []string
	}{
		{"W0-charter.md", []string{"W0-charter.md"}},
		{"W0.md", []string{"W0.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyState(tt.files)

			if result.State != StateW0Only {
				t.Errorf("Expected StateW0Only, got %s", result.State)
			}
			if len(result.W0Files) != 1 {
				t.Errorf("Expected 1 W0 file, got %d", len(result.W0Files))
			}
			if len(result.StatusFiles) != 0 {
				t.Errorf("Expected 0 STATUS files, got %d", len(result.StatusFiles))
			}
		})
	}
}

func TestClassifyState_StatusOnly(t *testing.T) {
	files := []string{"WAYFINDER-STATUS.md"}
	result := classifyState(files)

	if result.State != StateStatusOnly {
		t.Errorf("Expected StateStatusOnly, got %s", result.State)
	}
	if len(result.W0Files) != 0 {
		t.Errorf("Expected 0 W0 files, got %d", len(result.W0Files))
	}
	if len(result.StatusFiles) != 1 {
		t.Errorf("Expected 1 STATUS file, got %d", len(result.StatusFiles))
	}
}

func TestClassifyState_BothW0AndStatus(t *testing.T) {
	tests := []struct {
		name  string
		files []string
	}{
		{"W0-charter + STATUS", []string{"W0-charter.md", "WAYFINDER-STATUS.md"}},
		{"W0 + STATUS", []string{"W0.md", "WAYFINDER-STATUS.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyState(tt.files)

			if result.State != StateBothW0AndStatus {
				t.Errorf("Expected StateBothW0AndStatus, got %s", result.State)
			}
			if len(result.W0Files) != 1 {
				t.Errorf("Expected 1 W0 file, got %d", len(result.W0Files))
			}
			if len(result.StatusFiles) != 1 {
				t.Errorf("Expected 1 STATUS file, got %d", len(result.StatusFiles))
			}
		})
	}
}

func TestClassifyState_NonResumable(t *testing.T) {
	tests := []struct {
		name  string
		files []string
	}{
		{"random file", []string{"README.md"}},
		{"W0 + other", []string{"W0-charter.md", "D1-problem-validation.md"}},
		{"STATUS + other", []string{"WAYFINDER-STATUS.md", "D1-problem-validation.md"}},
		{"all + other", []string{"W0-charter.md", "WAYFINDER-STATUS.md", "D1-problem-validation.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyState(tt.files)

			if result.State != StateNonResumable {
				t.Errorf("Expected StateNonResumable for %v, got %s", tt.files, result.State)
			}
		})
	}
}

func TestIsW0File(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"W0-charter.md", "W0-charter.md", true},
		{"W0.md", "W0.md", true},
		{"w0-charter.md (lowercase)", "w0-charter.md", false},
		{"W0-CHARTER.MD (uppercase)", "W0-CHARTER.MD", false},
		{"README.md", "README.md", false},
		{"WAYFINDER-STATUS.md", "WAYFINDER-STATUS.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isW0File(tt.filename)
			if got != tt.want {
				t.Errorf("isW0File(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsStatusFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"WAYFINDER-STATUS.md", "WAYFINDER-STATUS.md", true},
		{"wayfinder-status.md (lowercase)", "wayfinder-status.md", false},
		{"STATUS.md", "STATUS.md", false},
		{"README.md", "README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStatusFile(tt.filename)
			if got != tt.want {
				t.Errorf("isStatusFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestScanDirectory_HiddenFileExclusion(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"W0-charter.md",
		"README.md",
		".git",
		".gitignore",
		".DS_Store",
	}

	for _, name := range testFiles {
		path := filepath.Join(tmpDir, name)
		if name == ".git" {
			// Create as directory
			if err := os.Mkdir(path, 0755); err != nil {
				t.Fatalf("Failed to create dir %s: %v", name, err)
			}
		} else {
			// Create as file
			if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
				t.Fatalf("Failed to create file %s: %v", name, err)
			}
		}
	}

	// Scan directory
	files, err := scanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanDirectory failed: %v", err)
	}

	// Should only return visible files (W0-charter.md, README.md)
	expectedCount := 2
	if len(files) != expectedCount {
		t.Errorf("Expected %d visible files, got %d: %v", expectedCount, len(files), files)
	}

	// Verify hidden files excluded
	for _, file := range files {
		if file == ".gitignore" || file == ".DS_Store" {
			t.Errorf("Hidden file %s should have been excluded", file)
		}
	}
}

func TestScanDirectory_PermissionDenied(t *testing.T) {
	// This test is skipped on most systems since we can't easily create permission-denied dirs
	t.Skip("Permission test requires special setup")

	// Would test:
	// tmpDir := createUnreadableDir()
	// _, err := scanDirectory(tmpDir)
	// if !errors.Is(err, ErrPermissionDenied) {
	//     t.Errorf("Expected ErrPermissionDenied, got %v", err)
	// }
}
