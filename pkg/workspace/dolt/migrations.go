package dolt

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GetAppliedMigrations returns all migrations applied for a component.
func (a *WorkspaceAdapter) GetAppliedMigrations(ctx context.Context, component string) ([]*Migration, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `
SELECT id, component, version, name, checksum, applied_at, applied_by, execution_time_ms, tables_created
FROM dolt_migrations
WHERE component = ?
ORDER BY version ASC
`

	rows, err := a.db.QueryContext(ctx, query, component)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	var migrations []*Migration
	for rows.Next() {
		m := &Migration{}
		err := rows.Scan(
			&m.ID,
			&m.Component,
			&m.Version,
			&m.Name,
			&m.Checksum,
			&m.AppliedAt,
			&m.AppliedBy,
			&m.ExecutionTimeMs,
			&m.TablesCreated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration: %w", err)
		}
		migrations = append(migrations, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migrations: %w", err)
	}

	return migrations, nil
}

// IsMigrationApplied checks if a specific migration version has been applied.
func (a *WorkspaceAdapter) IsMigrationApplied(ctx context.Context, component string, version int) (bool, error) {
	if a.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	query := `SELECT COUNT(*) FROM dolt_migrations WHERE component = ? AND version = ?`
	var count int
	err := a.db.QueryRowContext(ctx, query, component, version).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check migration: %w", err)
	}

	return count > 0, nil
}

// ApplyMigration applies a single migration within a transaction.
func (a *WorkspaceAdapter) ApplyMigration(ctx context.Context, component string, migration *MigrationFile, appliedBy string) (*MigrationResult, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	// Check if already applied
	applied, err := a.IsMigrationApplied(ctx, component, migration.Version)
	if err != nil {
		return nil, err
	}
	if applied {
		return nil, fmt.Errorf("%w: component=%s version=%d", ErrDuplicateMigration, component, migration.Version)
	}

	// Validate dependencies
	if err := a.validateMigrationDependencies(ctx, component, migration); err != nil {
		return nil, err
	}

	result := &MigrationResult{
		Migration: migration,
		Success:   false,
	}

	startTime := time.Now()

	// Execute migration in transaction
	err = a.ExecuteInTransaction(ctx, func(tx *sql.Tx) error {
		// Execute migration SQL
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("%w: %v", ErrMigrationFailed, err)
		}

		// Record migration in registry
		tablesJSON, _ := json.Marshal(migration.TablesCreated)
		execTime := int(time.Since(startTime).Milliseconds())

		query := `
INSERT INTO dolt_migrations (component, version, name, checksum, applied_by, execution_time_ms, tables_created)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
		_, err := tx.ExecContext(ctx, query,
			component,
			migration.Version,
			migration.Name,
			migration.Checksum,
			appliedBy,
			execTime,
			string(tablesJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		return nil
	})

	result.ExecutionTimeMs = int(time.Since(startTime).Milliseconds())

	if err != nil {
		result.Error = err
		return result, err
	}

	result.Success = true
	return result, nil
}

// ApplyMigrations applies multiple migrations in order.
func (a *WorkspaceAdapter) ApplyMigrations(ctx context.Context, component string, migrations []*MigrationFile, appliedBy string) ([]*MigrationResult, error) {
	var results []*MigrationResult

	for _, migration := range migrations {
		result, err := a.ApplyMigration(ctx, component, migration, appliedBy)
		results = append(results, result)

		if err != nil {
			// Stop on first failure
			return results, fmt.Errorf("migration %d failed: %w", migration.Version, err)
		}
	}

	return results, nil
}

// LoadMigrationFiles loads migration files from a directory.
func LoadMigrationFiles(migrationsDir string) ([]*MigrationFile, error) {
	// Pattern: {version}_{name}.sql (e.g., 001_initial_schema.sql)
	pattern := regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []*MigrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := pattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			// Skip files that don't match pattern
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid version in filename %s: %w", entry.Name(), err)
		}

		name := matches[2]
		filePath := filepath.Join(migrationsDir, entry.Name())

		// Read SQL content
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		// Compute checksum
		checksum := computeChecksum(content)

		// Parse tables created (simple approach: look for CREATE TABLE statements)
		tables := parseTablesCreated(string(content))

		migration := &MigrationFile{
			Version:       version,
			Name:          name,
			FilePath:      filePath,
			SQL:           string(content),
			Checksum:      checksum,
			TablesCreated: tables,
		}

		migrations = append(migrations, migration)
	}

	// Sort by version
	for i := 0; i < len(migrations); i++ {
		for j := i + 1; j < len(migrations); j++ {
			if migrations[i].Version > migrations[j].Version {
				migrations[i], migrations[j] = migrations[j], migrations[i]
			}
		}
	}

	return migrations, nil
}

// computeChecksum computes SHA256 checksum of migration content.
func computeChecksum(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", hash)
}

// parseTablesCreated extracts table names from CREATE TABLE statements.
func parseTablesCreated(sql string) []string {
	// Pattern: CREATE TABLE [IF NOT EXISTS] table_name
	pattern := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z0-9_]+)`)
	matches := pattern.FindAllStringSubmatch(sql, -1)

	var tables []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			table := strings.ToLower(match[1])
			if !seen[table] {
				tables = append(tables, table)
				seen[table] = true
			}
		}
	}

	return tables
}

// validateMigrationDependencies checks if migration dependencies are satisfied.
func (a *WorkspaceAdapter) validateMigrationDependencies(ctx context.Context, component string, migration *MigrationFile) error {
	if len(migration.DependsOn) == 0 {
		return nil
	}

	for _, depVersion := range migration.DependsOn {
		applied, err := a.IsMigrationApplied(ctx, component, depVersion)
		if err != nil {
			return err
		}
		if !applied {
			return fmt.Errorf("%w: migration %d depends on version %d which is not applied",
				ErrDependencyNotMet, migration.Version, depVersion)
		}
	}

	return nil
}

// GetUnappliedMigrations returns migrations that haven't been applied yet.
func (a *WorkspaceAdapter) GetUnappliedMigrations(ctx context.Context, component string, allMigrations []*MigrationFile) ([]*MigrationFile, error) {
	appliedMigrations, err := a.GetAppliedMigrations(ctx, component)
	if err != nil {
		return nil, err
	}

	// Build set of applied versions
	appliedVersions := make(map[int]bool)
	for _, m := range appliedMigrations {
		appliedVersions[m.Version] = true
	}

	// Filter unapplied
	var unapplied []*MigrationFile
	for _, m := range allMigrations {
		if !appliedVersions[m.Version] {
			unapplied = append(unapplied, m)
		}
	}

	return unapplied, nil
}

// VerifyMigrationChecksum verifies that a migration file matches the recorded checksum.
func (a *WorkspaceAdapter) VerifyMigrationChecksum(ctx context.Context, component string, migration *MigrationFile) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}

	query := `SELECT checksum FROM dolt_migrations WHERE component = ? AND version = ?`
	var recordedChecksum string
	err := a.db.QueryRowContext(ctx, query, component, migration.Version).Scan(&recordedChecksum)
	if err == sql.ErrNoRows {
		return nil // Not applied yet, no verification needed
	}
	if err != nil {
		return fmt.Errorf("failed to query migration checksum: %w", err)
	}

	if recordedChecksum != migration.Checksum {
		return fmt.Errorf("%w: component=%s version=%d expected=%s got=%s",
			ErrInvalidChecksum, component, migration.Version, recordedChecksum, migration.Checksum)
	}

	return nil
}

// GetMigrationTablesCreated returns the tables created by a migration.
func (a *WorkspaceAdapter) GetMigrationTablesCreated(ctx context.Context, component string, version int) ([]string, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `SELECT tables_created FROM dolt_migrations WHERE component = ? AND version = ?`
	var tablesJSON string
	err := a.db.QueryRowContext(ctx, query, component, version).Scan(&tablesJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: component=%s version=%d", ErrMigrationNotFound, component, version)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query migration tables: %w", err)
	}

	var tables []string
	if err := json.Unmarshal([]byte(tablesJSON), &tables); err != nil {
		return nil, fmt.Errorf("failed to parse tables JSON: %w", err)
	}

	return tables, nil
}

// GetAllComponentTables returns all tables owned by a component.
func (a *WorkspaceAdapter) GetAllComponentTables(ctx context.Context, component string) ([]string, error) {
	migrations, err := a.GetAppliedMigrations(ctx, component)
	if err != nil {
		return nil, err
	}

	tablesSet := make(map[string]bool)
	for _, m := range migrations {
		var tables []string
		if err := json.Unmarshal([]byte(m.TablesCreated), &tables); err != nil {
			continue
		}
		for _, table := range tables {
			tablesSet[table] = true
		}
	}

	var allTables []string
	for table := range tablesSet {
		allTables = append(allTables, table)
	}

	return allTables, nil
}
