package dolt

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// RegisterComponent registers a component in the component registry.
func (a *WorkspaceAdapter) RegisterComponent(ctx context.Context, info *ComponentInfo) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}

	// Validate prefix
	if err := validatePrefix(info.Prefix); err != nil {
		return err
	}

	// Check for prefix collision
	if err := a.checkPrefixCollision(ctx, info.Name, info.Prefix); err != nil {
		return err
	}

	query := `
INSERT INTO component_registry (name, version, prefix, status, manifest)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  version = VALUES(version),
  prefix = VALUES(prefix),
  status = VALUES(status),
  manifest = VALUES(manifest),
  updated_at = CURRENT_TIMESTAMP
`

	_, err := a.db.ExecContext(ctx, query,
		info.Name,
		info.Version,
		info.Prefix,
		info.Status,
		info.Manifest,
	)
	if err != nil {
		return fmt.Errorf("failed to register component: %w", err)
	}

	return nil
}

// GetComponent retrieves component information by name.
func (a *WorkspaceAdapter) GetComponent(ctx context.Context, name string) (*ComponentInfo, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `
SELECT name, version, prefix, installed_at, updated_at, status, manifest
FROM component_registry
WHERE name = ?
`

	info := &ComponentInfo{}
	err := a.db.QueryRowContext(ctx, query, name).Scan(
		&info.Name,
		&info.Version,
		&info.Prefix,
		&info.InstalledAt,
		&info.UpdatedAt,
		&info.Status,
		&info.Manifest,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrComponentNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query component: %w", err)
	}

	return info, nil
}

// ListComponents returns all registered components.
func (a *WorkspaceAdapter) ListComponents(ctx context.Context) ([]*ComponentInfo, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `
SELECT name, version, prefix, installed_at, updated_at, status, manifest
FROM component_registry
ORDER BY name
`

	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query components: %w", err)
	}
	defer rows.Close()

	var components []*ComponentInfo
	for rows.Next() {
		info := &ComponentInfo{}
		err := rows.Scan(
			&info.Name,
			&info.Version,
			&info.Prefix,
			&info.InstalledAt,
			&info.UpdatedAt,
			&info.Status,
			&info.Manifest,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan component: %w", err)
		}
		components = append(components, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating components: %w", err)
	}

	return components, nil
}

// UpdateComponentStatus updates the status of a component.
func (a *WorkspaceAdapter) UpdateComponentStatus(ctx context.Context, name string, status ComponentStatus) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}

	query := `UPDATE component_registry SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?`
	result, err := a.db.ExecContext(ctx, query, status, name)
	if err != nil {
		return fmt.Errorf("failed to update component status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrComponentNotFound, name)
	}

	return nil
}

// UnregisterComponent removes a component from the registry.
func (a *WorkspaceAdapter) UnregisterComponent(ctx context.Context, name string) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}

	// Check for dependencies
	deps, err := a.GetComponentDependents(ctx, name)
	if err != nil {
		return err
	}
	if len(deps) > 0 {
		return fmt.Errorf("cannot unregister component %s: other components depend on it: %v", name, deps)
	}

	return a.ExecuteInTransaction(ctx, func(tx *sql.Tx) error {
		// Delete component registry entry
		query := `DELETE FROM component_registry WHERE name = ?`
		result, err := tx.ExecContext(ctx, query, name)
		if err != nil {
			return fmt.Errorf("failed to delete component: %w", err)
		}

		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		if rows == 0 {
			return fmt.Errorf("%w: %s", ErrComponentNotFound, name)
		}

		// Delete migration records
		query = `DELETE FROM dolt_migrations WHERE component = ?`
		_, err = tx.ExecContext(ctx, query, name)
		if err != nil {
			return fmt.Errorf("failed to delete migrations: %w", err)
		}

		return nil
	})
}

// checkPrefixCollision checks if a prefix is already claimed by another component.
func (a *WorkspaceAdapter) checkPrefixCollision(ctx context.Context, componentName string, prefix string) error {
	query := `SELECT name FROM component_registry WHERE prefix = ? AND name != ?`
	var existingComponent string
	err := a.db.QueryRowContext(ctx, query, prefix, componentName).Scan(&existingComponent)
	if err == sql.ErrNoRows {
		return nil // No collision
	}
	if err != nil {
		return fmt.Errorf("failed to check prefix collision: %w", err)
	}

	return fmt.Errorf("%w: prefix '%s' is already claimed by component '%s'",
		ErrPrefixCollision, prefix, existingComponent)
}

// validatePrefix validates a component prefix.
func validatePrefix(prefix string) error {
	if prefix == "" {
		return fmt.Errorf("prefix cannot be empty")
	}

	// Check if prefix is reserved
	for _, reserved := range ReservedPrefixes {
		if strings.EqualFold(prefix, reserved) {
			return fmt.Errorf("%w: %s", ErrReservedPrefix, prefix)
		}
	}

	// Check format: lowercase alphanumeric + underscore, must end with underscore
	if !strings.HasSuffix(prefix, "_") {
		return fmt.Errorf("prefix must end with underscore: %s", prefix)
	}

	// Check characters (lowercase, alphanumeric, underscore only)
	for _, c := range prefix {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("prefix must contain only lowercase letters, numbers, and underscores: %s", prefix)
		}
	}

	// Check length (3-50 characters including underscore)
	// Minimum 3 ensures at least 2 chars + underscore (e.g., "wf_" not "a_")
	if len(prefix) < 3 || len(prefix) > 50 {
		return fmt.Errorf("prefix length must be 3-50 characters: %s (len=%d)", prefix, len(prefix))
	}

	return nil
}

// GetComponentDependents returns components that depend on the given component.
func (a *WorkspaceAdapter) GetComponentDependents(ctx context.Context, componentName string) ([]string, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	// Query for components that have this component in their dependencies
	// This requires parsing the manifest JSON field
	query := `
SELECT name
FROM component_registry
WHERE JSON_SEARCH(manifest, 'one', ?, NULL, '$.dependencies[*].name') IS NOT NULL
AND status = 'installed'
`

	rows, err := a.db.QueryContext(ctx, query, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to query component dependents: %w", err)
	}
	defer rows.Close()

	var dependents []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan dependent: %w", err)
		}
		dependents = append(dependents, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating dependents: %w", err)
	}

	return dependents, nil
}

// GetComponentByPrefix returns the component that owns the given table prefix.
func (a *WorkspaceAdapter) GetComponentByPrefix(ctx context.Context, prefix string) (*ComponentInfo, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `
SELECT name, version, prefix, installed_at, updated_at, status, manifest
FROM component_registry
WHERE prefix = ?
`

	info := &ComponentInfo{}
	err := a.db.QueryRowContext(ctx, query, prefix).Scan(
		&info.Name,
		&info.Version,
		&info.Prefix,
		&info.InstalledAt,
		&info.UpdatedAt,
		&info.Status,
		&info.Manifest,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: prefix %s", ErrComponentNotFound, prefix)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query component by prefix: %w", err)
	}

	return info, nil
}

// GetTablesByPrefix returns all tables with a given prefix.
func (a *WorkspaceAdapter) GetTablesByPrefix(ctx context.Context, prefix string) ([]string, error) {
	if a.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `SHOW TABLES LIKE ?`
	rows, err := a.db.QueryContext(ctx, query, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("failed to scan table: %w", err)
		}
		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tables: %w", err)
	}

	return tables, nil
}

// FindOrphanedTables finds tables with a component prefix that aren't registered in migrations.
func (a *WorkspaceAdapter) FindOrphanedTables(ctx context.Context, component string) ([]string, error) {
	// Get component info
	info, err := a.GetComponent(ctx, component)
	if err != nil {
		return nil, err
	}

	// Get all tables with component prefix
	actualTables, err := a.GetTablesByPrefix(ctx, info.Prefix)
	if err != nil {
		return nil, err
	}

	// Get registered tables from migrations
	registeredTables, err := a.GetAllComponentTables(ctx, component)
	if err != nil {
		return nil, err
	}

	// Build set of registered tables
	registered := make(map[string]bool)
	for _, table := range registeredTables {
		registered[table] = true
	}

	// Find orphans
	var orphans []string
	for _, table := range actualTables {
		if !registered[table] {
			orphans = append(orphans, table)
		}
	}

	return orphans, nil
}

// ValidatePrefixAvailable checks if a prefix is available for use.
func ValidatePrefixAvailable(ctx context.Context, a *WorkspaceAdapter, prefix string) error {
	if err := validatePrefix(prefix); err != nil {
		return err
	}

	// Check if already claimed (pass empty component name to check all)
	return a.checkPrefixCollision(ctx, "", prefix)
}

// ComponentManifest represents the structure of a component manifest.
type ComponentManifest struct {
	Name         string                   `json:"name"`
	Version      string                   `json:"version"`
	Description  string                   `json:"description"`
	Storage      ComponentStorage         `json:"storage"`
	Dependencies []ComponentDependency    `json:"dependencies,omitempty"`
	Migrations   []ComponentMigrationInfo `json:"migrations,omitempty"`
}

// ComponentStorage represents storage configuration in manifest.
type ComponentStorage struct {
	Engine string `json:"engine"`
	Prefix string `json:"prefix"`
}

// ComponentDependency represents a component dependency.
type ComponentDependency struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Required         bool     `json:"required"`
	TablesReferenced []string `json:"tables_referenced,omitempty"`
}

// ComponentMigrationInfo represents migration metadata in manifest.
type ComponentMigrationInfo struct {
	Version       int      `json:"version"`
	Name          string   `json:"name"`
	File          string   `json:"file"`
	Description   string   `json:"description,omitempty"`
	Checksum      string   `json:"checksum,omitempty"`
	DependsOn     []int    `json:"depends_on,omitempty"`
	TablesCreated []string `json:"tables_created,omitempty"`
}

// ParseManifest parses a component manifest from JSON.
func ParseManifest(manifestJSON string) (*ComponentManifest, error) {
	var manifest ComponentManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	return &manifest, nil
}

// ValidateManifest validates a component manifest.
func ValidateManifest(manifest *ComponentManifest) error {
	if manifest.Name == "" {
		return fmt.Errorf("manifest name cannot be empty")
	}
	if manifest.Version == "" {
		return fmt.Errorf("manifest version cannot be empty")
	}
	if manifest.Storage.Engine != "dolt" {
		return fmt.Errorf("unsupported storage engine: %s (only 'dolt' supported)", manifest.Storage.Engine)
	}
	if err := validatePrefix(manifest.Storage.Prefix); err != nil {
		return fmt.Errorf("invalid storage prefix: %w", err)
	}
	return nil
}
