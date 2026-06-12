package s7_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/s7"
)

const validPlan = `# Implementation Plan: My Cool Project

**Project**: My Cool Project

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| auth-login | User login flow | 2d | login_test.go |
| auth-logout | User logout flow | 1d | logout_test.go |
`

func TestParseS7Plan_Valid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "S7-plan.md")
	if err := os.WriteFile(path, []byte(validPlan), 0600); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	project, features, err := s7.ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if project != "My Cool Project" {
		t.Errorf("project = %q, want %q", project, "My Cool Project")
	}
	if len(features) != 2 {
		t.Fatalf("features len = %d, want 2", len(features))
	}
	if features[0].ID != "auth-login" {
		t.Errorf("features[0].ID = %q, want %q", features[0].ID, "auth-login")
	}
	if features[1].ID != "auth-logout" {
		t.Errorf("features[1].ID = %q, want %q", features[1].ID, "auth-logout")
	}
}

func TestParseS7Plan_FileNotFound(t *testing.T) {
	t.Parallel()
	_, _, err := s7.ParseS7Plan("/nonexistent/path/plan.md")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseS7Plan_NoFeatureTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	content := "# Implementation Plan: X\n\nNo feature table here.\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	_, _, err := s7.ParseS7Plan(path)
	if err == nil {
		t.Error("expected error when feature table is missing")
	}
}

func TestParseS7Plan_SkipsExampleRows(t *testing.T) {
	t.Parallel()
	plan := `# Implementation Plan: Test

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| feature-1 | example | 1d | test.go |
| feature-2 | example | 1d | test.go |
| real-feature | real thing | 2d | real_test.go |
`
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte(plan), 0600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	_, features, err := s7.ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("features len = %d, want 1 (example rows should be skipped)", len(features))
	}
	if features[0].ID != "real-feature" {
		t.Errorf("features[0].ID = %q, want real-feature", features[0].ID)
	}
}
