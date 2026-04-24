package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestValidateD2Content_FileNotFound(t *testing.T) {
	st := &status.Status{
		StartedAt: time.Now(),
	}

	// Use non-existent directory
	projectDir := "/tmp/nonexistent-project-12345"

	err := validateD2Content(projectDir, st)
	if err == nil {
		t.Fatal("expected error for missing D2 file, got nil")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}

func TestValidateD2Content_FileTooLarge(t *testing.T) {
	// Create temp dir with huge D2 file
	tmpDir := t.TempDir()
	d2Path := filepath.Join(tmpDir, "D2-existing-solutions.md")

	// Create file larger than 1MB
	largeContent := strings.Repeat("a", maxFileSizeBytes+1)
	if err := os.WriteFile(d2Path, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	st := &status.Status{
		StartedAt: time.Now(),
	}

	err := validateD2Content(tmpDir, st)
	if err == nil {
		t.Fatal("expected error for large file, got nil")
	}

	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestValidateD2Content_MissingOverlap(t *testing.T) {
	// Use test fixture
	projectDir := "testdata"

	st := &status.Status{
		StartedAt: time.Now(),
	}

	// Copy missing-overlap fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "d2-missing-overlap.md")
	dst := filepath.Join(tmpDir, "D2-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateD2Content(tmpDir, st)
	if err == nil {
		t.Fatal("expected error for missing overlap, got nil")
	}

	if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("expected 'overlap' error, got: %v", err)
	}
}

func TestValidateD2Content_MissingSearchMethodology(t *testing.T) {
	// Use test fixture with overlap < 100% but no search methodology
	projectDir := "testdata"

	st := &status.Status{
		StartedAt: time.Now(),
	}

	// Copy missing-methodology fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "d2-missing-methodology.md")
	dst := filepath.Join(tmpDir, "D2-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateD2Content(tmpDir, st)
	if err == nil {
		t.Fatal("expected error for missing search methodology, got nil")
	}

	if !strings.Contains(err.Error(), "methodology") {
		t.Errorf("expected 'methodology' error, got: %v", err)
	}
}

func TestValidateD2Content_SearchMethodologyOptional(t *testing.T) {
	// With 100% overlap, search methodology is optional
	projectDir := "testdata"

	st := &status.Status{
		StartedAt: time.Now(),
	}

	// Copy valid-100 fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "d2-valid-100.md")
	dst := filepath.Join(tmpDir, "D2-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateD2Content(tmpDir, st)
	if err != nil {
		t.Errorf("expected no error for 100%% overlap without methodology, got: %v", err)
	}
}

func TestValidateD2Content_TooShort(t *testing.T) {
	// File with < 200 words
	projectDir := "testdata"

	st := &status.Status{
		StartedAt: time.Now(),
	}

	// Copy too-short fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "d2-too-short.md")
	dst := filepath.Join(tmpDir, "D2-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateD2Content(tmpDir, st)
	if err == nil {
		t.Fatal("expected error for short file, got nil")
	}

	if !strings.Contains(err.Error(), "too short") {
		t.Errorf("expected 'too short' error, got: %v", err)
	}
}

func TestValidateD2Content_LegacyProject(t *testing.T) {
	// Project started before gate deployment date
	legacyTime, _ := time.Parse(time.RFC3339, "2026-01-15T00:00:00Z")
	st := &status.Status{
		StartedAt: legacyTime, // Before 2026-01-20
	}

	// Use non-existent directory - should bypass due to legacy status
	projectDir := "/tmp/nonexistent-legacy-project"

	err := validateD2Content(projectDir, st)
	if err != nil {
		t.Errorf("expected no error for legacy project, got: %v", err)
	}
}

func TestValidateD2Content_Valid(t *testing.T) {
	// Valid D2 file with all required fields
	projectDir := "testdata"

	st := &status.Status{
		StartedAt: time.Now(),
	}

	// Copy valid-87 fixture to temp dir
	tmpDir := t.TempDir()
	src := filepath.Join(projectDir, "d2-valid-87.md")
	dst := filepath.Join(tmpDir, "D2-existing-solutions.md")
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(dst, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = validateD2Content(tmpDir, st)
	if err != nil {
		t.Errorf("expected no error for valid D2, got: %v", err)
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

func TestIsLegacyProject_Before(t *testing.T) {
	legacyTime, _ := time.Parse(time.RFC3339, "2026-01-15T00:00:00Z")
	st := &status.Status{
		StartedAt: legacyTime, // Before gate deployment
	}

	if !isLegacyProject(st) {
		t.Error("expected true for project before gate deployment")
	}
}

func TestIsLegacyProject_After(t *testing.T) {
	futureTime, _ := time.Parse(time.RFC3339, "2026-01-25T00:00:00Z")
	st := &status.Status{
		StartedAt: futureTime, // After gate deployment
	}

	if isLegacyProject(st) {
		t.Error("expected false for project after gate deployment")
	}
}

// TestIsLegacyProject_ParseError no longer needed since StartedAt is time.Time, not string

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
