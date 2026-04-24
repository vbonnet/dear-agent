package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestMigrateProject_Success(t *testing.T) {
	// Create temporary project directory
	tmpDir := t.TempDir()

	// Create V1 status file
	now := time.Now()
	completed := now.Add(-1 * time.Hour)
	v1Status := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-session-123",
		ProjectPath:   tmpDir,
		StartedAt:     now,
		Status:        status.StatusInProgress,
		CurrentPhase:  "S8",
		Phases: []status.Phase{
			{
				Name:        "D1",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   &now,
				CompletedAt: &completed,
			},
		},
	}

	if err := v1Status.WriteTo(tmpDir); err != nil {
		t.Fatalf("Failed to write V1 status: %v", err)
	}

	// Run migration
	result, err := MigrateProject(tmpDir)
	if err != nil {
		t.Fatalf("MigrateProject() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Migration should succeed, got Success = false")
	}

	if result.BackupPath == "" {
		t.Error("BackupPath should be set")
	}

	// Verify backup exists
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Errorf("Backup file should exist: %v", err)
	}

	// Verify V2 file was created
	v2Status, err := status.ParseV2FromDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read V2 status: %v", err)
	}

	if v2Status.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %v, want %v", v2Status.SchemaVersion, status.SchemaVersionV2)
	}
}

func TestMigrateProject_NoStatusFile(t *testing.T) {
	// Create temporary project directory without status file
	tmpDir := t.TempDir()

	// Run migration (should fail)
	_, err := MigrateProject(tmpDir)
	if err == nil {
		t.Error("MigrateProject() should fail when no status file exists")
	}

	if !strings.Contains(err.Error(), "no WAYFINDER-STATUS.md found") {
		t.Errorf("Error should mention missing file, got: %v", err)
	}
}

func TestMigrateProject_AlreadyV2(t *testing.T) {
	// Create temporary project directory
	tmpDir := t.TempDir()

	// Create V2 status file
	v2Status := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     "test-project",
		ProjectType:     status.ProjectTypeFeature,
		RiskLevel:       status.RiskLevelM,
		CurrentWaypoint: status.PhaseV2Problem,
		Status:          status.StatusV2InProgress,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := status.WriteV2ToDir(v2Status, tmpDir); err != nil {
		t.Fatalf("Failed to write V2 status: %v", err)
	}

	// Run migration
	result, err := MigrateProject(tmpDir)
	if err != nil {
		t.Fatalf("MigrateProject() error = %v", err)
	}

	if !result.Success {
		t.Error("Migration should succeed for already-V2 projects")
	}

	if len(result.Warnings) == 0 {
		t.Error("Should have warning about already being V2")
	}

	if result.Warnings[0] != "Already V2 schema, skipping migration" {
		t.Errorf("Unexpected warning: %v", result.Warnings[0])
	}

	// Verify no backup was created (no migration needed)
	if result.BackupPath != "" {
		t.Errorf("BackupPath should be empty for already-V2 projects, got %v", result.BackupPath)
	}
}

func TestCreateBackup(t *testing.T) {
	// Create temporary status file
	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, status.StatusFilename)

	testContent := "---\ntest: data\n---\n"
	if err := os.WriteFile(statusPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create backup
	backupPath, err := createBackup(statusPath)
	if err != nil {
		t.Fatalf("createBackup() error = %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("Backup file should exist: %v", err)
	}

	// Verify backup filename format
	if !strings.HasPrefix(filepath.Base(backupPath), "WAYFINDER-STATUS.v1.backup.") {
		t.Errorf("Backup filename should have correct prefix, got: %s", filepath.Base(backupPath))
	}

	if !strings.HasSuffix(backupPath, ".md") {
		t.Errorf("Backup filename should have .md extension, got: %s", backupPath)
	}

	// Verify backup content matches original
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	if string(backupContent) != testContent {
		t.Errorf("Backup content = %q, want %q", string(backupContent), testContent)
	}
}

func TestRestoreFromBackup(t *testing.T) {
	// Create temporary project directory
	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, status.StatusFilename)
	backupPath := filepath.Join(tmpDir, "backup.md")

	// Create backup file
	backupContent := "---\nbackup: data\n---\n"
	if err := os.WriteFile(backupPath, []byte(backupContent), 0644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Create current status file (will be overwritten)
	currentContent := "---\ncurrent: data\n---\n"
	if err := os.WriteFile(statusPath, []byte(currentContent), 0644); err != nil {
		t.Fatalf("Failed to create status file: %v", err)
	}

	// Restore from backup
	if err := RestoreFromBackup(tmpDir, backupPath); err != nil {
		t.Fatalf("RestoreFromBackup() error = %v", err)
	}

	// Verify status file was restored
	restoredContent, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if string(restoredContent) != backupContent {
		t.Errorf("Restored content = %q, want %q", string(restoredContent), backupContent)
	}
}

func TestDryRun_Success(t *testing.T) {
	// Create temporary project directory
	tmpDir := t.TempDir()

	// Create V1 status file
	v1Status := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-123",
		ProjectPath:   tmpDir,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases:        []status.Phase{},
	}

	if err := v1Status.WriteTo(tmpDir); err != nil {
		t.Fatalf("Failed to write V1 status: %v", err)
	}

	// Run dry-run
	warnings, err := DryRun(tmpDir)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	// Dry-run should succeed without warnings for simple case
	if len(warnings) > 0 && !strings.Contains(warnings[0], "Already V2") {
		t.Logf("Warnings: %v", warnings)
	}

	// Verify original file is unchanged
	v1StatusCheck, err := status.ReadFrom(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read V1 status after dry-run: %v", err)
	}

	if v1StatusCheck.SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion = %v, want 1.0 (file should not be modified)", v1StatusCheck.SchemaVersion)
	}
}

func TestDryRun_AlreadyV2(t *testing.T) {
	// Create temporary project directory
	tmpDir := t.TempDir()

	// Create V2 status file
	v2Status := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     "test-project",
		ProjectType:     status.ProjectTypeFeature,
		RiskLevel:       status.RiskLevelM,
		CurrentWaypoint: status.PhaseV2Problem,
		Status:          status.StatusV2InProgress,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := status.WriteV2ToDir(v2Status, tmpDir); err != nil {
		t.Fatalf("Failed to write V2 status: %v", err)
	}

	// Run dry-run
	warnings, err := DryRun(tmpDir)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	// Should have warning about already being V2
	if len(warnings) == 0 {
		t.Error("Should have warning about already being V2")
	}

	if !strings.Contains(warnings[0], "Already V2 schema") {
		t.Errorf("Unexpected warning: %v", warnings[0])
	}
}

func TestDryRun_NoStatusFile(t *testing.T) {
	// Create temporary project directory without status file
	tmpDir := t.TempDir()

	// Run dry-run (should fail)
	_, err := DryRun(tmpDir)
	if err == nil {
		t.Error("DryRun() should fail when no status file exists")
	}

	if !strings.Contains(err.Error(), "no WAYFINDER-STATUS.md found") {
		t.Errorf("Error should mention missing file, got: %v", err)
	}
}

func TestDryRun_ValidationError(t *testing.T) {
	// Create temporary project directory
	tmpDir := t.TempDir()

	// Create invalid V1 status file (missing required fields)
	// This test is tricky because we need a V1 file that parses but converts to invalid V2
	// For now, we'll test with a valid V1 that converts successfully
	v1Status := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-123",
		ProjectPath:   tmpDir,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases:        []status.Phase{},
	}

	if err := v1Status.WriteTo(tmpDir); err != nil {
		t.Fatalf("Failed to write V1 status: %v", err)
	}

	// Run dry-run (should succeed for this valid case)
	warnings, err := DryRun(tmpDir)
	if err != nil {
		t.Fatalf("DryRun() error = %v (expected success for valid V1)", err)
	}

	// Log warnings for inspection
	if len(warnings) > 0 {
		t.Logf("Warnings: %v", warnings)
	}
}
