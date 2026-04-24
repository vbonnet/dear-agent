package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/converter"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// MigrationResult contains the result of a migration operation
type MigrationResult struct {
	ProjectPath  string
	Success      bool
	BackupPath   string
	ErrorMessage string
	Warnings     []string
}

// MigrateProject migrates a single project from V1 to V2
// Creates backup before migration
func MigrateProject(projectPath string) (*MigrationResult, error) {
	result := &MigrationResult{
		ProjectPath: projectPath,
		Success:     false,
		Warnings:    []string{},
	}

	// Check if status file exists
	statusPath := filepath.Join(projectPath, status.StatusFilename)
	if _, err := os.Stat(statusPath); err != nil {
		if os.IsNotExist(err) {
			return result, fmt.Errorf("no WAYFINDER-STATUS.md found in %s", projectPath)
		}
		return result, fmt.Errorf("failed to check status file: %w", err)
	}

	// Detect current schema version
	schemaVersion, err := status.DetectSchemaVersion(statusPath)
	if err != nil {
		return result, fmt.Errorf("failed to detect schema version: %w", err)
	}

	// Check if already V2
	if schemaVersion == status.SchemaVersionV2 {
		result.Success = true
		result.Warnings = append(result.Warnings, "Already V2 schema, skipping migration")
		return result, nil
	}

	// Read V1 status
	v1Status, err := status.ReadFrom(projectPath)
	if err != nil {
		return result, fmt.Errorf("failed to read V1 status: %w", err)
	}

	// Create backup
	backupPath, err := createBackup(statusPath)
	if err != nil {
		return result, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backupPath

	// Convert V1 to V2
	v2Status, err := converter.ConvertV1ToV2(v1Status)
	if err != nil {
		return result, fmt.Errorf("failed to convert V1 to V2: %w", err)
	}

	// Validate V2 status
	if err := status.ValidateV2(v2Status); err != nil {
		return result, fmt.Errorf("V2 validation failed: %w", err)
	}

	// Write V2 status
	if err := status.WriteV2ToDir(v2Status, projectPath); err != nil {
		return result, fmt.Errorf("failed to write V2 status: %w", err)
	}

	result.Success = true
	return result, nil
}

// createBackup creates a timestamped backup of the status file
// Returns the backup file path
func createBackup(statusPath string) (string, error) {
	// Read original file
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return "", fmt.Errorf("failed to read status file: %w", err)
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	dir := filepath.Dir(statusPath)
	backupFilename := fmt.Sprintf("WAYFINDER-STATUS.v1.backup.%s.md", timestamp)
	backupPath := filepath.Join(dir, backupFilename)

	// Write backup
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

// RestoreFromBackup restores a status file from backup
func RestoreFromBackup(projectPath, backupPath string) error {
	// Read backup file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Write to status file
	statusPath := filepath.Join(projectPath, status.StatusFilename)
	if err := os.WriteFile(statusPath, data, 0644); err != nil {
		return fmt.Errorf("failed to restore status file: %w", err)
	}

	return nil
}

// DryRun performs a dry-run migration without modifying files
// Returns validation errors if any
func DryRun(projectPath string) ([]string, error) {
	var warnings []string

	// Check if status file exists
	statusPath := filepath.Join(projectPath, status.StatusFilename)
	if _, err := os.Stat(statusPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no WAYFINDER-STATUS.md found in %s", projectPath)
		}
		return nil, fmt.Errorf("failed to check status file: %w", err)
	}

	// Detect current schema version
	schemaVersion, err := status.DetectSchemaVersion(statusPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect schema version: %w", err)
	}

	// Check if already V2
	if schemaVersion == status.SchemaVersionV2 {
		warnings = append(warnings, "Already V2 schema, no migration needed")
		return warnings, nil
	}

	// Read V1 status
	v1Status, err := status.ReadFrom(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read V1 status: %w", err)
	}

	// Convert V1 to V2 (dry-run, no writes)
	v2Status, err := converter.ConvertV1ToV2(v1Status)
	if err != nil {
		return nil, fmt.Errorf("failed to convert V1 to V2: %w", err)
	}

	// Validate V2 status
	if err := status.ValidateV2(v2Status); err != nil {
		return nil, fmt.Errorf("V2 validation failed: %w", err)
	}

	// Add informational warnings
	if len(v1Status.Phases) > len(v2Status.WaypointHistory) {
		warnings = append(warnings, fmt.Sprintf("Some V1 phases removed in V2: %d -> %d phases", len(v1Status.Phases), len(v2Status.WaypointHistory)))
	}

	return warnings, nil
}
