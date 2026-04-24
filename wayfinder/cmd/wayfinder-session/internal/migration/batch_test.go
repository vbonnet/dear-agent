package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestMigrateAll_EmptyWorkspace(t *testing.T) {
	// Create temporary empty workspace
	tmpDir := t.TempDir()

	options := BatchMigrationOptions{
		WorkspaceRoot: tmpDir,
		DryRun:        false,
		Parallel:      false,
	}

	report, err := MigrateAll(options)
	if err != nil {
		t.Fatalf("MigrateAll() error = %v", err)
	}

	if report.TotalProjects != 0 {
		t.Errorf("TotalProjects = %d, want 0", report.TotalProjects)
	}
}

func TestMigrateAll_SingleV1Project(t *testing.T) {
	// Create temporary workspace with one V1 project
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Create V1 status file
	v1Status := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-123",
		ProjectPath:   projectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases:        []status.Phase{},
	}

	if err := v1Status.WriteTo(projectPath); err != nil {
		t.Fatalf("Failed to write V1 status: %v", err)
	}

	options := BatchMigrationOptions{
		WorkspaceRoot: tmpDir,
		DryRun:        false,
		Parallel:      false,
	}

	report, err := MigrateAll(options)
	if err != nil {
		t.Fatalf("MigrateAll() error = %v", err)
	}

	if report.TotalProjects != 1 {
		t.Errorf("TotalProjects = %d, want 1", report.TotalProjects)
	}

	if report.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", report.SuccessCount)
	}

	if report.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", report.FailureCount)
	}

	// Verify backup was created
	backups, err := filepath.Glob(filepath.Join(projectPath, "WAYFINDER-STATUS.v1.backup.*.md"))
	if err != nil {
		t.Fatalf("Failed to check for backups: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("Expected 1 backup file, found %d", len(backups))
	}

	// Verify V2 file was created
	v2Status, err := status.ParseV2FromDir(projectPath)
	if err != nil {
		t.Fatalf("Failed to read V2 status: %v", err)
	}

	if v2Status.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %v, want %v", v2Status.SchemaVersion, status.SchemaVersionV2)
	}
}

func TestMigrateAll_MultipleProjects(t *testing.T) {
	// Create temporary workspace with multiple V1 projects
	tmpDir := t.TempDir()

	projectCount := 3
	for i := 0; i < projectCount; i++ {
		projectPath := filepath.Join(tmpDir, fmt.Sprintf("project-%d", i))
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			t.Fatalf("Failed to create project dir: %v", err)
		}

		// Create V1 status file
		v1Status := &status.Status{
			SchemaVersion: "1.0",
			SessionID:     fmt.Sprintf("test-%d", i),
			ProjectPath:   projectPath,
			StartedAt:     time.Now(),
			Status:        status.StatusInProgress,
			CurrentPhase:  "D1",
			Phases:        []status.Phase{},
		}

		if err := v1Status.WriteTo(projectPath); err != nil {
			t.Fatalf("Failed to write V1 status: %v", err)
		}
	}

	options := BatchMigrationOptions{
		WorkspaceRoot: tmpDir,
		DryRun:        false,
		Parallel:      false,
	}

	report, err := MigrateAll(options)
	if err != nil {
		t.Fatalf("MigrateAll() error = %v", err)
	}

	if report.TotalProjects != projectCount {
		t.Errorf("TotalProjects = %d, want %d", report.TotalProjects, projectCount)
	}

	if report.SuccessCount != projectCount {
		t.Errorf("SuccessCount = %d, want %d", report.SuccessCount, projectCount)
	}

	if report.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", report.FailureCount)
	}
}

func TestMigrateAll_DryRun(t *testing.T) {
	// Create temporary workspace with one V1 project
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Create V1 status file
	v1Status := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-123",
		ProjectPath:   projectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases:        []status.Phase{},
	}

	if err := v1Status.WriteTo(projectPath); err != nil {
		t.Fatalf("Failed to write V1 status: %v", err)
	}

	options := BatchMigrationOptions{
		WorkspaceRoot: tmpDir,
		DryRun:        true,
		Parallel:      false,
	}

	report, err := MigrateAll(options)
	if err != nil {
		t.Fatalf("MigrateAll() error = %v", err)
	}

	if report.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", report.SuccessCount)
	}

	// Verify no backup was created (dry-run)
	backups, err := filepath.Glob(filepath.Join(projectPath, "WAYFINDER-STATUS.v1.backup.*.md"))
	if err != nil {
		t.Fatalf("Failed to check for backups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("Expected 0 backup files in dry-run, found %d", len(backups))
	}

	// Verify V1 file still exists (not modified)
	v1StatusCheck, err := status.ReadFrom(projectPath)
	if err != nil {
		t.Fatalf("Failed to read V1 status after dry-run: %v", err)
	}
	if v1StatusCheck.SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion = %v, want 1.0 (file should not be modified in dry-run)", v1StatusCheck.SchemaVersion)
	}
}

func TestMigrateAll_SkipsV2Projects(t *testing.T) {
	// Create temporary workspace with mixed V1 and V2 projects
	tmpDir := t.TempDir()

	// Create V1 project
	v1ProjectPath := filepath.Join(tmpDir, "v1-project")
	if err := os.MkdirAll(v1ProjectPath, 0755); err != nil {
		t.Fatalf("Failed to create V1 project dir: %v", err)
	}

	v1Status := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-v1",
		ProjectPath:   v1ProjectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases:        []status.Phase{},
	}

	if err := v1Status.WriteTo(v1ProjectPath); err != nil {
		t.Fatalf("Failed to write V1 status: %v", err)
	}

	// Create V2 project
	v2ProjectPath := filepath.Join(tmpDir, "v2-project")
	if err := os.MkdirAll(v2ProjectPath, 0755); err != nil {
		t.Fatalf("Failed to create V2 project dir: %v", err)
	}

	v2Status := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     "v2-project",
		ProjectType:     status.ProjectTypeFeature,
		RiskLevel:       status.RiskLevelM,
		CurrentWaypoint: status.PhaseV2Problem,
		Status:          status.StatusV2InProgress,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := status.WriteV2ToDir(v2Status, v2ProjectPath); err != nil {
		t.Fatalf("Failed to write V2 status: %v", err)
	}

	options := BatchMigrationOptions{
		WorkspaceRoot: tmpDir,
		DryRun:        false,
		Parallel:      false,
	}

	report, err := MigrateAll(options)
	if err != nil {
		t.Fatalf("MigrateAll() error = %v", err)
	}

	// Only V1 project should be counted
	if report.TotalProjects != 1 {
		t.Errorf("TotalProjects = %d, want 1 (V2 projects should be excluded)", report.TotalProjects)
	}

	if report.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", report.SuccessCount)
	}
}

func TestMigrateAll_Parallel(t *testing.T) {
	// Create temporary workspace with multiple V1 projects
	tmpDir := t.TempDir()

	projectCount := 5
	for i := 0; i < projectCount; i++ {
		projectPath := filepath.Join(tmpDir, fmt.Sprintf("project-%d", i))
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			t.Fatalf("Failed to create project dir: %v", err)
		}

		// Create V1 status file
		v1Status := &status.Status{
			SchemaVersion: "1.0",
			SessionID:     fmt.Sprintf("test-%d", i),
			ProjectPath:   projectPath,
			StartedAt:     time.Now(),
			Status:        status.StatusInProgress,
			CurrentPhase:  "D1",
			Phases:        []status.Phase{},
		}

		if err := v1Status.WriteTo(projectPath); err != nil {
			t.Fatalf("Failed to write V1 status: %v", err)
		}
	}

	options := BatchMigrationOptions{
		WorkspaceRoot: tmpDir,
		DryRun:        false,
		Parallel:      true,
		MaxWorkers:    2,
	}

	report, err := MigrateAll(options)
	if err != nil {
		t.Fatalf("MigrateAll() error = %v", err)
	}

	if report.TotalProjects != projectCount {
		t.Errorf("TotalProjects = %d, want %d", report.TotalProjects, projectCount)
	}

	if report.SuccessCount != projectCount {
		t.Errorf("SuccessCount = %d, want %d", report.SuccessCount, projectCount)
	}

	if report.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", report.FailureCount)
	}

	// Verify all projects were migrated
	for i := 0; i < projectCount; i++ {
		projectPath := filepath.Join(tmpDir, fmt.Sprintf("project-%d", i))
		v2Status, err := status.ParseV2FromDir(projectPath)
		if err != nil {
			t.Errorf("Failed to read V2 status for project-%d: %v", i, err)
			continue
		}

		if v2Status.SchemaVersion != status.SchemaVersionV2 {
			t.Errorf("project-%d: SchemaVersion = %v, want %v", i, v2Status.SchemaVersion, status.SchemaVersionV2)
		}
	}
}

func TestFindV1Projects(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested project structure
	v1Project1 := filepath.Join(tmpDir, "workspace1", "project1")
	v1Project2 := filepath.Join(tmpDir, "workspace1", "project2")
	v2Project := filepath.Join(tmpDir, "workspace2", "project3")

	for _, path := range []string{v1Project1, v1Project2, v2Project} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	// Create V1 projects
	for _, path := range []string{v1Project1, v1Project2} {
		v1Status := &status.Status{
			SchemaVersion: "1.0",
			SessionID:     "test",
			ProjectPath:   path,
			StartedAt:     time.Now(),
			Status:        status.StatusInProgress,
			CurrentPhase:  "D1",
			Phases:        []status.Phase{},
		}
		if err := v1Status.WriteTo(path); err != nil {
			t.Fatalf("Failed to write V1 status: %v", err)
		}
	}

	// Create V2 project
	v2Status := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     "project3",
		ProjectType:     status.ProjectTypeFeature,
		RiskLevel:       status.RiskLevelM,
		CurrentWaypoint: status.PhaseV2Problem,
		Status:          status.StatusV2InProgress,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := status.WriteV2ToDir(v2Status, v2Project); err != nil {
		t.Fatalf("Failed to write V2 status: %v", err)
	}

	// Find V1 projects
	projects, err := findV1Projects(tmpDir)
	if err != nil {
		t.Fatalf("findV1Projects() error = %v", err)
	}

	// Should only find 2 V1 projects
	if len(projects) != 2 {
		t.Errorf("findV1Projects() found %d projects, want 2", len(projects))
	}

	// Verify projects are V1
	for _, path := range projects {
		if path == v2Project {
			t.Errorf("findV1Projects() included V2 project: %s", path)
		}
	}
}

func TestBatchMigrationReport_PrintReport(t *testing.T) {
	report := &BatchMigrationReport{
		TotalProjects:   5,
		SuccessCount:    3,
		FailureCount:    1,
		SkippedCount:    1,
		SuccessProjects: []string{"/path/to/project1", "/path/to/project2", "/path/to/project3"},
		FailedProjects: map[string]string{
			"/path/to/project4": "conversion error",
		},
		SkippedProjects: map[string]string{
			"/path/to/project5": "already V2",
		},
		Warnings: map[string][]string{
			"/path/to/project1": {"warning 1", "warning 2"},
		},
	}

	// This test just ensures PrintReport doesn't panic
	// Output is tested manually
	report.PrintReport()
}
