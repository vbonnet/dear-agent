package s7

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPlan = `# Implementation Plan: My Project

**Project**: My Project

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| F1 | First feature | 2h | unit tests |
| F2 | Second feature | 4h | integration tests |
`

func writePlan(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "S7-plan-*.md")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	_ = f.Close()
	return f.Name()
}

func TestParseS7Plan_Valid(t *testing.T) {
	t.Parallel()
	path := writePlan(t, validPlan)

	project, features, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if project != "My Project" {
		t.Errorf("project = %q, want %q", project, "My Project")
	}
	if len(features) != 2 {
		t.Fatalf("features len = %d, want 2", len(features))
	}
	if features[0].ID != "F1" || features[1].ID != "F2" {
		t.Errorf("feature IDs = %v/%v, want F1/F2", features[0].ID, features[1].ID)
	}
	if features[0].Estimate != "2h" {
		t.Errorf("F1 Estimate = %q, want 2h", features[0].Estimate)
	}
}

func TestParseS7Plan_SkipsExampleRows(t *testing.T) {
	t.Parallel()
	plan := `# Implementation Plan: Test

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| feature-1 | example | 1h | none |
| feature-2 | example | 1h | none |
| F1 | real feature | 2h | tests |
`
	path := writePlan(t, plan)
	_, features, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	for _, f := range features {
		if f.ID == "feature-1" || f.ID == "feature-2" {
			t.Errorf("example row %q should be skipped", f.ID)
		}
	}
	if len(features) != 1 {
		t.Errorf("features len = %d, want 1", len(features))
	}
}

func TestParseS7Plan_NoFeatures(t *testing.T) {
	t.Parallel()
	plan := `# Implementation Plan: Test

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| feature-1 | example | 1h | none |
`
	path := writePlan(t, plan)
	_, _, err := ParseS7Plan(path)
	if err == nil {
		t.Error("expected error for plan with only example rows, got nil")
	}
}

func TestParseS7Plan_MissingSection(t *testing.T) {
	t.Parallel()
	path := writePlan(t, "# Just a header\n\nNo feature tracking here.\n")
	_, _, err := ParseS7Plan(path)
	if err == nil {
		t.Error("expected error for plan without Feature Tracking section, got nil")
	}
}

func TestParseS7Plan_NotFound(t *testing.T) {
	t.Parallel()
	_, _, err := ParseS7Plan("/nonexistent/path/plan.md")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestExtractProjectName_Header(t *testing.T) {
	t.Parallel()
	content := "# Implementation Plan: My Cool Project\n"
	got := extractProjectName(content)
	if got != "My Cool Project" {
		t.Errorf("extractProjectName = %q, want %q", got, "My Cool Project")
	}
}

func TestExtractProjectName_Bold(t *testing.T) {
	t.Parallel()
	content := "**Project**: Awesome Proj\n"
	got := extractProjectName(content)
	if got != "Awesome Proj" {
		t.Errorf("extractProjectName = %q, want %q", got, "Awesome Proj")
	}
}

func TestExtractProjectName_Default(t *testing.T) {
	t.Parallel()
	got := extractProjectName("no project name here")
	if got != "unknown-project" {
		t.Errorf("extractProjectName = %q, want unknown-project", got)
	}
}

func TestFindS7Plan_Found(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	planPath := filepath.Join(dir, "S7-plan.md")
	if err := os.WriteFile(planPath, []byte(validPlan), 0600); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	// Change to dir — t.Chdir is 1.24+, use Chdir directly with cleanup.
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	got, err := FindS7Plan()
	if err != nil {
		t.Fatalf("FindS7Plan: %v", err)
	}
	if !strings.HasSuffix(got, "S7-plan.md") {
		t.Errorf("FindS7Plan = %q, want path ending in S7-plan.md", got)
	}
}

func TestFindS7Plan_NotFound(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	_, err := FindS7Plan()
	if err == nil {
		t.Error("FindS7Plan: expected error when no plan file exists, got nil")
	}
}
