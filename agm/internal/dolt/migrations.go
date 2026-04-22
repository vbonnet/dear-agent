// Package dolt provides dolt functionality.
package dolt

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

//go:embed migrations/001_initial_schema.sql
var migration001 string

//go:embed migrations/002_messages_table.sql
var migration002 string

//go:embed migrations/003_add_tool_calls.sql
var migration003 string

//go:embed migrations/004_add_session_tags.sql
var migration004 string

//go:embed migrations/005_add_message_embeddings.sql
var migration005 string

//go:embed migrations/006_add_performance_indexes.sql
var migration006 string

//go:embed migrations/007_add_session_hierarchy.sql
var migration007 string

//go:embed migrations/008_add_permission_mode.sql
var migration008 string

//go:embed migrations/009_rename_agent_to_harness.sql
var migration009 string

//go:embed migrations/010_add_is_test.sql
var migration010 string

//go:embed migrations/011_add_worktree_tracking.sql
var migration011 string

//go:embed migrations/012_add_frecency.sql
var migration012 string

//go:embed migrations/013_add_context_usage.sql
var migration013 string

//go:embed migrations/014_add_monitors.sql
var migration014 string

//go:embed migrations/015_session_logs.sql
var migration015 string

//go:embed migrations/016_artifacts.sql
var migration016 string

// Migration represents a single database migration
type Migration struct {
	Version       int
	Name          string
	SQL           string
	Checksum      string
	TablesCreated []string
	// PreConditionSQL is an optional query that must return at least one row for
	// the migration SQL to execute. If it returns zero rows the SQL is skipped
	// but the migration is still recorded as applied, making DDL migrations
	// idempotent against schema states that already satisfy the target shape.
	// The checksum is computed from SQL only, so PreConditionSQL changes do not
	// invalidate previously recorded migrations.
	PreConditionSQL string
}

// AllMigrations returns all AGM migrations in order
func AllMigrations() []Migration {
	return []Migration{
		{
			Version:       1,
			Name:          "initial_schema",
			SQL:           migration001,
			Checksum:      computeChecksum(migration001),
			TablesCreated: []string{"agm_sessions"},
		},
		{
			Version:       2,
			Name:          "messages_table",
			SQL:           migration002,
			Checksum:      computeChecksum(migration002),
			TablesCreated: []string{"agm_messages"},
		},
		{
			Version:       3,
			Name:          "add_tool_calls",
			SQL:           migration003,
			Checksum:      computeChecksum(migration003),
			TablesCreated: []string{"agm_tool_calls"},
		},
		{
			Version:       4,
			Name:          "add_session_tags",
			SQL:           migration004,
			Checksum:      computeChecksum(migration004),
			TablesCreated: []string{"agm_session_tags"},
		},
		{
			Version:       5,
			Name:          "add_message_embeddings",
			SQL:           migration005,
			Checksum:      computeChecksum(migration005),
			TablesCreated: []string{"agm_message_embeddings"},
		},
		{
			Version:       6,
			Name:          "add_performance_indexes",
			SQL:           migration006,
			Checksum:      computeChecksum(migration006),
			TablesCreated: []string{}, // No new tables, only indexes
		},
		{
			Version:       7,
			Name:          "add_session_hierarchy",
			SQL:           migration007,
			Checksum:      computeChecksum(migration007),
			TablesCreated: []string{}, // No new tables, adds column to agm_sessions
		},
		{
			Version:       8,
			Name:          "add_permission_mode",
			SQL:           migration008,
			Checksum:      computeChecksum(migration008),
			TablesCreated: []string{}, // No new tables, adds columns to agm_sessions
		},
		{
			Version:       9,
			Name:          "rename_agent_to_harness",
			SQL:           migration009,
			Checksum:      computeChecksum(migration009),
			TablesCreated: []string{}, // No new tables, renames agent column to harness
			// Only run the ALTER TABLE if the 'agent' column still exists.
			// A stale binary may have caused migration 009 to be skipped in the
			// registry while the column was already renamed by a newer binary,
			// or vice-versa. This guard makes the migration idempotent.
			PreConditionSQL: `
				SELECT COLUMN_NAME
				FROM information_schema.COLUMNS
				WHERE TABLE_SCHEMA = DATABASE()
				  AND TABLE_NAME = 'agm_sessions'
				  AND COLUMN_NAME = 'agent'
			`,
		},
		{
			Version:       10,
			Name:          "add_is_test",
			SQL:           migration010,
			Checksum:      computeChecksum(migration010),
			TablesCreated: []string{}, // No new tables, adds column to agm_sessions
			// Only run the ALTER TABLE if the 'is_test' column doesn't exist yet.
			// Returns a row when column is missing (precondition met = run SQL).
			PreConditionSQL: `
				SELECT 1 WHERE NOT EXISTS (
					SELECT 1 FROM information_schema.COLUMNS
					WHERE TABLE_SCHEMA = DATABASE()
					  AND TABLE_NAME = 'agm_sessions'
					  AND COLUMN_NAME = 'is_test'
				)
			`,
		},
		{
			Version:       11,
			Name:          "add_worktree_tracking",
			SQL:           migration011,
			Checksum:      computeChecksum(migration011),
			TablesCreated: []string{"agm_worktrees"},
		},
		{
			Version:       12,
			Name:          "add_frecency",
			SQL:           migration012,
			Checksum:      computeChecksum(migration012),
			TablesCreated: []string{},
			PreConditionSQL: `
				SELECT 1 WHERE NOT EXISTS (
					SELECT 1 FROM information_schema.COLUMNS
					WHERE TABLE_SCHEMA = DATABASE()
					  AND TABLE_NAME = 'agm_sessions'
					  AND COLUMN_NAME = 'access_count'
				)
			`,
		},
		{
			Version:       13,
			Name:          "add_context_usage",
			SQL:           migration013,
			Checksum:      computeChecksum(migration013),
			TablesCreated: []string{},
			PreConditionSQL: `
				SELECT 1 WHERE NOT EXISTS (
					SELECT 1 FROM information_schema.COLUMNS
					WHERE TABLE_SCHEMA = DATABASE()
					  AND TABLE_NAME = 'agm_sessions'
					  AND COLUMN_NAME = 'context_total_tokens'
				)
			`,
		},
		{
			Version:       14,
			Name:          "add_monitors",
			SQL:           migration014,
			Checksum:      computeChecksum(migration014),
			TablesCreated: []string{},
			PreConditionSQL: `
				SELECT 1 WHERE NOT EXISTS (
					SELECT 1 FROM information_schema.COLUMNS
					WHERE TABLE_SCHEMA = DATABASE()
					  AND TABLE_NAME = 'agm_sessions'
					  AND COLUMN_NAME = 'monitors'
				)
			`,
		},
		{
			Version:       15,
			Name:          "session_logs",
			SQL:           migration015,
			Checksum:      computeChecksum(migration015),
			TablesCreated: []string{"agm_session_logs"},
		},
		{
			Version:       16,
			Name:          "artifacts",
			SQL:           migration016,
			Checksum:      computeChecksum(migration016),
			TablesCreated: []string{"agm_artifacts"},
		},
	}
}

// ApplyMigrations applies all pending migrations to the database
func (a *Adapter) ApplyMigrations() error {
	if a.migrationsApplied {
		return nil // Already applied in this adapter instance
	}

	// Ensure migration registry tables exist
	if err := a.ensureMigrationRegistry(); err != nil {
		return fmt.Errorf("failed to ensure migration registry: %w", err)
	}

	// Get applied migrations
	appliedVersions, err := a.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Apply pending migrations
	for _, migration := range AllMigrations() {
		if contains(appliedVersions, migration.Version) {
			// Validate checksum matches
			if err := a.validateMigrationChecksum(migration); err != nil {
				return fmt.Errorf("migration %d checksum mismatch: %w", migration.Version, err)
			}
			continue
		}

		// Apply migration
		if err := a.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}

	a.migrationsApplied = true
	return nil
}

// ensureMigrationRegistry creates migration registry tables if they don't exist
func (a *Adapter) ensureMigrationRegistry() error {
	// Create agm_migrations table (stores applied migrations)
	createMigrations := `
		CREATE TABLE IF NOT EXISTS agm_migrations (
			id INT AUTO_INCREMENT PRIMARY KEY,
			component VARCHAR(255) NOT NULL,
			version INT NOT NULL,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(128) NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			applied_by VARCHAR(255),
			execution_time_ms INT,
			tables_created JSON,
			UNIQUE KEY (component, version),
			INDEX idx_component (component),
			INDEX idx_applied_at (applied_at)
		)
	`

	if _, err := a.conn.Exec(createMigrations); err != nil {
		return fmt.Errorf("failed to create agm_migrations table: %w", err)
	}

	// Create component_registry table (stores installed components)
	createRegistry := `
		CREATE TABLE IF NOT EXISTS component_registry (
			name VARCHAR(255) PRIMARY KEY,
			version VARCHAR(50) NOT NULL,
			prefix VARCHAR(50) NOT NULL UNIQUE,
			installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status VARCHAR(20) DEFAULT 'installed',
			manifest JSON,
			INDEX idx_status (status),
			INDEX idx_prefix (prefix)
		)
	`

	if _, err := a.conn.Exec(createRegistry); err != nil {
		return fmt.Errorf("failed to create component_registry table: %w", err)
	}

	// Register AGM component if not already registered
	registerAGM := `
		INSERT INTO component_registry (name, version, prefix, manifest)
		VALUES ('agm', '1.4.0', 'agm_', '{"name": "agm", "version": "1.4.0", "description": "Agent-Generated Messaging"}')
		ON DUPLICATE KEY UPDATE version=VALUES(version), manifest=VALUES(manifest)
	`

	if _, err := a.conn.Exec(registerAGM); err != nil {
		return fmt.Errorf("failed to register AGM component: %w", err)
	}

	return nil
}

// getAppliedMigrations returns a list of applied migration versions for AGM
func (a *Adapter) getAppliedMigrations() ([]int, error) {
	query := `
		SELECT version FROM agm_migrations
		WHERE component = 'agm'
		ORDER BY version ASC
	`

	rows, err := a.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return versions, nil
}

// validateMigrationChecksum verifies that an applied migration has the correct checksum
func (a *Adapter) validateMigrationChecksum(migration Migration) error {
	query := `
		SELECT checksum FROM agm_migrations
		WHERE component = 'agm' AND version = ?
	`

	var storedChecksum string
	err := a.conn.QueryRow(query, migration.Version).Scan(&storedChecksum)
	if err != nil {
		return fmt.Errorf("failed to get stored checksum: %w", err)
	}

	if storedChecksum != migration.Checksum {
		return fmt.Errorf("checksum mismatch (stored: %s, expected: %s)", storedChecksum, migration.Checksum)
	}

	return nil
}

// applyMigration applies a single migration within a transaction
func (a *Adapter) applyMigration(migration Migration) error {
	// Evaluate optional pre-condition before opening a transaction.
	// If the pre-condition query returns zero rows the migration SQL is not
	// executed (the target schema state is already satisfied), but the
	// migration is still recorded as applied so future runs skip it cleanly.
	skipSQL := false
	if migration.PreConditionSQL != "" {
		rows, err := a.conn.Query(migration.PreConditionSQL)
		if err != nil {
			return fmt.Errorf("failed to evaluate pre-condition: %w", err)
		}
		hasRows := rows.Next()
		rows.Close()
		if !hasRows {
			skipSQL = true
		}
	}

	// Start transaction
	tx, err := a.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	// Measure execution time
	startTime := time.Now()

	// Execute migration SQL (skip when pre-condition determined it is a no-op)
	if !skipSQL {
		if _, err := tx.Exec(migration.SQL); err != nil {
			return fmt.Errorf("failed to execute migration SQL: %w", err)
		}
	}

	executionTime := time.Since(startTime).Milliseconds()

	// Convert tables created to JSON
	tablesJSON := "[]"
	if len(migration.TablesCreated) > 0 {
		tablesJSON = fmt.Sprintf("[\"%s\"]", joinStrings(migration.TablesCreated, "\", \""))
	}

	// Record migration in registry
	insertMigration := `
		INSERT INTO agm_migrations (component, version, name, checksum, applied_by, execution_time_ms, tables_created)
		VALUES ('agm', ?, ?, ?, 'agm-1.4.0', ?, ?)
	`

	_, err = tx.Exec(insertMigration, migration.Version, migration.Name, migration.Checksum, executionTime, tablesJSON)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// computeChecksum computes SHA256 checksum of migration SQL
func computeChecksum(sql string) string {
	hash := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(hash[:])
}

// Helper functions

func contains(slice []int, val int) bool {
	return slices.Contains(slice, val)
}

func joinStrings(slice []string, sep string) string {
	if len(slice) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(slice[0])
	for i := 1; i < len(slice); i++ {
		builder.WriteString(sep)
		builder.WriteString(slice[i])
	}
	return builder.String()
}
