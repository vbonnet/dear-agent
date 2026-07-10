package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateResearchContent_FileNotFound(t *testing.T) {
	// Use non-existent directory
	projectDir := "/tmp/nonexistent-project-12345"

	err := validateResearchContent(projectDir)
	if err == nil {
		t.Fatal("expected error for missing RESEARCH file, got nil")
		return
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}

func TestValidateResearchContent_FileTooLarge(t *testing.T) {
	// Create temp dir with huge RESEARCH file
	tmpDir := t.TempDir()
	researchPath := filepath.Join(tmpDir, "RESEARCH-existing-solutions.md")

	// Create file larger than 1MB
	largeContent := strings.Repeat("a", maxFileSizeBytes+1)
	if err := os.WriteFile(researchPath, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateResearchContent(tmpDir)
	if err == nil {
		t.Fatal("expected error for large file, got nil")
		return
	}

	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestValidateResearchContent_MissingOverlap(t *testing.T) {
	// Use test fixture
	projectDir := "testdata"

	// Copy missing-overlap fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "research-missing-overlap.md")
	dst := filepath.Join(tmpDir, "RESEARCH-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateResearchContent(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing overlap, got nil")
		return
	}

	if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("expected 'overlap' error, got: %v", err)
	}
}

func TestValidateResearchContent_MissingSearchMethodology(t *testing.T) {
	// Use test fixture with overlap < 100% but no search methodology
	projectDir := "testdata"

	// Copy missing-methodology fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "research-missing-methodology.md")
	dst := filepath.Join(tmpDir, "RESEARCH-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateResearchContent(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing search methodology, got nil")
		return
	}

	if !strings.Contains(err.Error(), "methodology") {
		t.Errorf("expected 'methodology' error, got: %v", err)
	}
}

func TestValidateResearchContent_SearchMethodologyOptional(t *testing.T) {
	// With 100% overlap, search methodology is optional
	projectDir := "testdata"

	// Copy valid-100 fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "research-valid-100.md")
	dst := filepath.Join(tmpDir, "RESEARCH-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateResearchContent(tmpDir)
	if err != nil {
		t.Errorf("expected no error for 100%% overlap without methodology, got: %v", err)
	}
}

func TestValidateResearchContent_TooShort(t *testing.T) {
	// File with < 200 words
	projectDir := "testdata"

	// Copy too-short fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "research-too-short.md")
	dst := filepath.Join(tmpDir, "RESEARCH-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateResearchContent(tmpDir)
	if err == nil {
		t.Fatal("expected error for short file, got nil")
		return
	}

	if !strings.Contains(err.Error(), "too short") {
		t.Errorf("expected 'too short' error, got: %v", err)
	}
}

func TestValidateResearchContent_Valid(t *testing.T) {
	// Valid RESEARCH file with all required fields
	projectDir := "testdata"

	// Copy valid-87 fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "research-valid-87.md")
	dst := filepath.Join(tmpDir, "RESEARCH-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateResearchContent(tmpDir)
	if err != nil {
		t.Errorf("expected no error for valid RESEARCH, got: %v", err)
	}
}

func TestExtractOverlapPercentage_Valid(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "standard format",
			content:  "## Analysis\n\nOverlap: 87%\n\nDetails...",
			expected: 87,
		},
		{
			name:     "no space",
			content:  "Overlap:100%",
			expected: 100,
		},
		{
			name:     "multiple spaces",
			content:  "Overlap:    0%",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overlap, err := extractOverlapPercentage(tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if overlap != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, overlap)
			}
		})
	}
}

func TestExtractOverlapPercentage_Missing(t *testing.T) {
	content := "## Analysis\n\nNo overlap field here\n"
	_, err := extractOverlapPercentage(content)
	if err == nil {
		t.Fatal("expected error for missing overlap, got nil")
	}
}

func TestHasSearchMethodology(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "has methodology",
			content:  "## Search methodology\n\nDetailed search...",
			expected: true,
		},
		{
			name:     "capitalized",
			content:  "## Search Methodology\n\nDetails...",
			expected: true,
		},
		{
			name:     "inline mention",
			content:  "The Search methodology section shows...",
			expected: true,
		},
		{
			name:     "missing",
			content:  "## Analysis\n\nNo search section...",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSearchMethodology(tt.content)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
