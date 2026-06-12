package s7

import (
	"os"
	"path/filepath"
	"testing"
)

func writePlan(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "S7-plan.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

const validPlan = `# Implementation Plan: Test Project

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| F-001 | Implement login | 2d | Auth smoke test passes |
| F-002 | Add logout | 1d | Session cleared on logout |
`

func TestParseS7Plan_ValidPlan(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, validPlan)

	project, features, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if project != "Test Project" {
		t.Errorf("project = %q, want Test Project", project)
	}
	if len(features) != 2 {
		t.Fatalf("want 2 features, got %d", len(features))
	}
	if features[0].ID != "F-001" {
		t.Errorf("features[0].ID = %q, want F-001", features[0].ID)
	}
	if features[1].ID != "F-002" {
		t.Errorf("features[1].ID = %q, want F-002", features[1].ID)
	}
	if features[0].Estimate != "2d" {
		t.Errorf("features[0].Estimate = %q, want 2d", features[0].Estimate)
	}
}

func TestParseS7Plan_ExampleRowsSkipped(t *testing.T) {
	plan := `# Implementation Plan: My App

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| feature-1 | Example one | 1d | Check |
| feature-2 | Example two | 2d | Check |
| F-100 | Real feature | 3d | Real check |
`
	dir := t.TempDir()
	path := writePlan(t, dir, plan)

	_, features, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("want 1 feature (example rows skipped), got %d: %+v", len(features), features)
	}
	if features[0].ID != "F-100" {
		t.Errorf("features[0].ID = %q, want F-100", features[0].ID)
	}
}

func TestParseS7Plan_NoFeatureTrackingSection(t *testing.T) {
	plan := `# Implementation Plan: Missing Section

No feature tracking here.
`
	dir := t.TempDir()
	path := writePlan(t, dir, plan)

	_, _, err := ParseS7Plan(path)
	if err == nil {
		t.Error("expected error when Feature Tracking section is absent")
	}
}

func TestParseS7Plan_AllExampleRows_Error(t *testing.T) {
	plan := `# Implementation Plan: Example Only

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| feature-1 | Placeholder | 1d | TBD |
| feature-2 | Placeholder | 2d | TBD |
`
	dir := t.TempDir()
	path := writePlan(t, dir, plan)

	_, _, err := ParseS7Plan(path)
	if err == nil {
		t.Error("expected error when only example rows are present")
	}
}

func TestParseS7Plan_FileNotFound(t *testing.T) {
	_, _, err := ParseS7Plan("/nonexistent/path/S7-plan.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseS7Plan_ProjectFromBoldSyntax(t *testing.T) {
	plan := `**Project**: Bold Project Name

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| F-001 | Thing | 1d | Done |
`
	dir := t.TempDir()
	path := writePlan(t, dir, plan)

	project, _, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if project != "Bold Project Name" {
		t.Errorf("project = %q, want Bold Project Name", project)
	}
}

func TestParseS7Plan_DefaultProjectName(t *testing.T) {
	plan := `# Implementation Plan: [Project Name]

## Feature Tracking

| Feature ID | Description | Estimate | Verification |
|------------|-------------|----------|--------------|
| F-001 | Thing | 1d | Done |
`
	dir := t.TempDir()
	path := writePlan(t, dir, plan)

	project, _, err := ParseS7Plan(path)
	if err != nil {
		t.Fatalf("ParseS7Plan: %v", err)
	}
	if project != "unknown-project" {
		t.Errorf("project = %q, want unknown-project for placeholder header", project)
	}
}
