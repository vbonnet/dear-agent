package s7

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractProjectName_FromHeader(t *testing.T) {
	content := "# Implementation Plan: My Cool Project\n\nsome content"
	got := extractProjectName(content)
	if got != "My Cool Project" {
		t.Errorf("extractProjectName header = %q, want %q", got, "My Cool Project")
	}
}

func TestExtractProjectName_FromProjectField(t *testing.T) {
	content := "**Project**: Widget Builder\n\nsome content"
	got := extractProjectName(content)
	if got != "Widget Builder" {
		t.Errorf("extractProjectName project field = %q, want %q", got, "Widget Builder")
	}
}

func TestExtractProjectName_Default(t *testing.T) {
	got := extractProjectName("no headers here")
	if got != "unknown-project" {
		t.Errorf("extractProjectName default = %q, want %q", got, "unknown-project")
	}
}

func TestParseS7Plan_ValidPlan(t *testing.T) {
	content := `# Implementation Plan: Test Project

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| F-01 | First feature | 2h | Manual test |
| F-02 | Second feature | 3h | Auto test |
`
	path := filepath.Join(t.TempDir(), "S7-plan.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	project, features, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if project != "Test Project" {
		t.Errorf("project = %q, want %q", project, "Test Project")
	}
	if len(features) != 2 {
		t.Errorf("features count = %d, want 2", len(features))
	}
	if features[0].ID != "F-01" {
		t.Errorf("features[0].ID = %q, want %q", features[0].ID, "F-01")
	}
	if features[1].ID != "F-02" {
		t.Errorf("features[1].ID = %q, want %q", features[1].ID, "F-02")
	}
}

func TestParseS7Plan_SkipsExampleRows(t *testing.T) {
	content := `# Implementation Plan: Skip Test

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| feature-1 | Example 1 | 1h | N/A |
| feature-2 | Example 2 | 1h | N/A |
| F-real | Real feature | 2h | Unit tests |
`
	path := filepath.Join(t.TempDir(), "S7-plan.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, features, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if len(features) != 1 {
		t.Errorf("features count = %d, want 1 (example rows skipped)", len(features))
	}
	if features[0].ID != "F-real" {
		t.Errorf("features[0].ID = %q, want %q", features[0].ID, "F-real")
	}
}

func TestParseS7Plan_MissingTable(t *testing.T) {
	content := "# My Plan\n\nNo table here."
	path := filepath.Join(t.TempDir(), "S7-plan.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseS7Plan(path); err == nil {
		t.Error("ParseS7Plan should return error for missing feature table")
	}
}

func TestParseS7Plan_NonexistentFile(t *testing.T) {
	if _, _, err := ParseS7Plan("/nonexistent/path.md"); err == nil {
		t.Error("ParseS7Plan should return error for nonexistent file")
	}
}
