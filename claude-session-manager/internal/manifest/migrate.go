package manifest

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// MigrateV1ToV2 migrates a v1 manifest to v2 format
// Acquires lock before migration to prevent concurrent migrations
// Creates .v1.bak backup before writing v2
// Idempotent: skips if .v1.bak already exists
func MigrateV1ToV2(path string) error {
	// CRITICAL: Acquire lock BEFORE migration
	// Prevents race condition if two processes load same v1 manifest
	if err := AcquireLock(path); err != nil {
		return fmt.Errorf("cannot acquire lock for migration: %w", err)
	}
	defer ReleaseLock(path)

	// Read v1 manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var v1 ManifestV1
	if err := yaml.Unmarshal(data, &v1); err != nil {
		return err
	}

	// Check if backup already exists (idempotency)
	backupPath := path + ".v1.bak"
	if _, err := os.Stat(backupPath); err == nil {
		// Backup exists, migration already done
		// This can happen if migration succeeded but load failed
		logMigration("SKIPPED", path, errors.New("backup already exists"))
		return nil
	}

	// Backup original (atomic write via fileutil)
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Convert to v2
	v2 := &Manifest{
		SchemaVersion: SchemaVersion,
		SessionID:     v1.SessionID,
		Name:          v1.Tmux.SessionName, // Use tmux session name as manifest name
		CreatedAt:     v1.CreatedAt,
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Empty = active/stopped (status computed dynamically)
		Context: Context{
			Project: v1.Worktree.Path, // Migrate worktree path to context project
			Purpose: "",               // Not in v1
			Tags:    nil,              // Not in v1
			Notes:   "",               // Not in v1
		},
		Tmux: Tmux{
			SessionName: v1.Tmux.SessionName,
		},
	}

	// Special case: if v1 status was "archived", set lifecycle to "archived"
	if v1.Status == StatusArchived {
		v2.Lifecycle = LifecycleArchived
	}

	// Save v2 using Write() which validates and uses atomic write
	if err := Write(path, v2); err != nil {
		// Migration failed, remove backup to allow retry
		os.Remove(backupPath)
		logMigration("FAILED", path, err)
		return fmt.Errorf("failed to save v2: %w", err)
	}

	// Log success
	logMigration("SUCCESS", path, nil)

	return nil
}

// MigrateV2ToV3 migrates a v2 manifest to v3 format
// Acquires lock before migration to prevent concurrent migrations
// Creates .v2.bak backup before writing v3
// Idempotent: skips if .v2.bak already exists
func MigrateV2ToV3(path string) error {
	// CRITICAL: Acquire lock BEFORE migration
	// Prevents race condition if two processes load same v2 manifest
	if err := AcquireLock(path); err != nil {
		return fmt.Errorf("cannot acquire lock for migration: %w", err)
	}
	defer ReleaseLock(path)

	// Read v2 manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var v2 Manifest
	if err := yaml.Unmarshal(data, &v2); err != nil {
		return err
	}

	// Check if backup already exists (idempotency)
	backupPath := path + ".v2.bak"
	if _, err := os.Stat(backupPath); err == nil {
		// Backup exists, migration already done
		// This can happen if migration succeeded but load failed
		logMigration("SKIPPED", path, errors.New("backup already exists"))
		return nil
	}

	// Backup original
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Convert to v3
	v3 := &ManifestV3{
		// Copy all v2 fields
		SchemaVersion: "3.0", // Bump version
		SessionID:     v2.SessionID,
		Name:          v2.Name,
		CreatedAt:     v2.CreatedAt,
		UpdatedAt:     time.Now(), // Update timestamp
		Lifecycle:     v2.Lifecycle,
		Context:       v2.Context,
		Claude:        v2.Claude,
		Tmux:          v2.Tmux,

		// Add v3 fields - default values for migrated sessions
		Agent:        "claude",        // Assume existing sessions are Claude
		AgentHistory: []AgentSwitch{}, // Empty array, not nil
	}

	// Validate v3 manifest before writing
	if err := v3.Validate(); err != nil {
		// Migration failed, remove backup to allow retry
		os.Remove(backupPath)
		logMigration("FAILED", path, err)
		return fmt.Errorf("v3 validation failed: %w", err)
	}

	// Save v3 using WriteV3() which validates and uses atomic write
	if err := WriteV3(path, v3); err != nil {
		// Migration failed, remove backup to allow retry
		os.Remove(backupPath)
		logMigration("FAILED", path, err)
		return fmt.Errorf("failed to save v3: %w", err)
	}

	// Log success
	logMigration("SUCCESS", path, nil)

	return nil
}

// detectVersion reads a manifest file and determines its schema version
// Returns "1.0", "2.0", "3.0", or error
func detectVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Parse just the schema_version field
	var versionCheck struct {
		SchemaVersion string `yaml:"schema_version"`
	}

	if err := yaml.Unmarshal(data, &versionCheck); err != nil {
		return "", err
	}

	// Default to v1 if schema_version is missing (early manifests)
	if versionCheck.SchemaVersion == "" {
		return "1.0", nil
	}

	return versionCheck.SchemaVersion, nil
}

// logMigration logs migration events to ~/.csm/logs/migration.log
// TODO: Implement proper logging when D3.2 Log Rotation is complete
func logMigration(status string, path string, err error) {
	// For now, just print to stderr
	// Will be replaced with proper file logging in D3.2
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MIGRATION %s] %s: %v\n", status, path, err)
	} else {
		fmt.Fprintf(os.Stderr, "[MIGRATION %s] %s\n", status, path)
	}
}
