// Package registry provides component registry and database.
package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver
)

// Registry manages the Corpus Callosum schema registry
type Registry struct {
	db            *sql.DB
	workspaceName string
	registryPath  string
}

// Schema represents a registered component schema
type Schema struct {
	ID            int    `json:"id"`
	Component     string `json:"component"`
	Version       string `json:"version"`
	Compatibility string `json:"compatibility"`
	SchemaJSON    string `json:"schema_json"`
	CreatedAt     int64  `json:"created_at"`
}

// Component represents component metadata
type Component struct {
	Component     string `json:"component"`
	Description   string `json:"description"`
	LatestVersion string `json:"latest_version"`
	InstalledAt   int64  `json:"installed_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS cc_schemas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    component TEXT NOT NULL,
    version TEXT NOT NULL,
    compatibility TEXT NOT NULL DEFAULT 'backward',
    schema_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(component, version)
);

CREATE INDEX IF NOT EXISTS idx_component ON cc_schemas(component);
CREATE INDEX IF NOT EXISTS idx_component_version ON cc_schemas(component, version);

CREATE TABLE IF NOT EXISTS cc_components (
    component TEXT PRIMARY KEY,
    description TEXT,
    latest_version TEXT NOT NULL,
    installed_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS cc_query_cache (
    query_hash TEXT PRIMARY KEY,
    result_json TEXT NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_expires ON cc_query_cache(expires_at);
`

// New creates a new registry instance
func New(workspace string) (*Registry, error) {
	registryPath, err := getRegistryPath(workspace)
	if err != nil {
		return nil, err
	}

	// Ensure directory exists
	dir := filepath.Dir(registryPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create registry directory: %w", err)
	}

	db, err := sql.Open("sqlite3", registryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &Registry{
		db:            db,
		workspaceName: workspace,
		registryPath:  registryPath,
	}, nil
}

// Close closes the database connection
func (r *Registry) Close() error {
	return r.db.Close()
}

// GetWorkspace returns the workspace name
func (r *Registry) GetWorkspace() string {
	return r.workspaceName
}

// GetPath returns the registry database path
func (r *Registry) GetPath() string {
	return r.registryPath
}

// RegisterSchema registers a component schema
func (r *Registry) RegisterSchema(component, version, compatibility string, schemaJSON map[string]interface{}) error {
	schemaBytes, err := json.Marshal(schemaJSON)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	now := time.Now().Unix() * 1000 // milliseconds

	ctx := context.Background()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // rollback after commit is expected to fail

	// Insert/update schema
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO cc_schemas (component, version, compatibility, schema_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, component, version, compatibility, string(schemaBytes), now)
	if err != nil {
		return fmt.Errorf("failed to insert schema: %w", err)
	}

	// Get description from schema if present
	description := ""
	if desc, ok := schemaJSON["description"].(string); ok {
		description = desc
	}

	// Update component metadata
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cc_components (component, description, latest_version, installed_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(component) DO UPDATE SET
			description = excluded.description,
			latest_version = excluded.latest_version,
			updated_at = excluded.updated_at
	`, component, description, version, now, now)
	if err != nil {
		return fmt.Errorf("failed to update component: %w", err)
	}

	return tx.Commit()
}

// GetSchema retrieves a schema by component and version
func (r *Registry) GetSchema(component, version string) (*Schema, error) {
	query := `
		SELECT id, component, version, compatibility, schema_json, created_at
		FROM cc_schemas
		WHERE component = ?
	`
	args := []interface{}{component}

	if version != "" {
		query += " AND version = ?"
		args = append(args, version)
	} else {
		// Get latest version (use id as tiebreaker)
		query += " ORDER BY created_at DESC, id DESC LIMIT 1"
	}

	var schema Schema
	ctx := context.Background()
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&schema.ID,
		&schema.Component,
		&schema.Version,
		&schema.Compatibility,
		&schema.SchemaJSON,
		&schema.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("schema not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query schema: %w", err)
	}

	return &schema, nil
}

// ListComponents returns all registered components
func (r *Registry) ListComponents() ([]Component, error) {
	ctx := context.Background()
	rows, err := r.db.QueryContext(ctx, `
		SELECT component, description, latest_version, installed_at, updated_at
		FROM cc_components
		ORDER BY component
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query components: %w", err)
	}
	defer rows.Close()

	var components []Component
	for rows.Next() {
		var comp Component
		if err := rows.Scan(&comp.Component, &comp.Description, &comp.LatestVersion, &comp.InstalledAt, &comp.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan component: %w", err)
		}
		components = append(components, comp)
	}

	return components, rows.Err()
}

// GetComponent retrieves component metadata
func (r *Registry) GetComponent(component string) (*Component, error) {
	var comp Component
	ctx := context.Background()
	err := r.db.QueryRowContext(ctx, `
		SELECT component, description, latest_version, installed_at, updated_at
		FROM cc_components
		WHERE component = ?
	`, component).Scan(&comp.Component, &comp.Description, &comp.LatestVersion, &comp.InstalledAt, &comp.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("component not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query component: %w", err)
	}

	return &comp, nil
}

// ListVersions returns all versions for a component
func (r *Registry) ListVersions(component string) ([]Schema, error) {
	ctx := context.Background()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, component, version, compatibility, schema_json, created_at
		FROM cc_schemas
		WHERE component = ?
		ORDER BY created_at DESC
	`, component)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	var schemas []Schema
	for rows.Next() {
		var schema Schema
		if err := rows.Scan(&schema.ID, &schema.Component, &schema.Version, &schema.Compatibility, &schema.SchemaJSON, &schema.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan schema: %w", err)
		}
		schemas = append(schemas, schema)
	}

	return schemas, rows.Err()
}

// UnregisterSchema soft-deletes a schema
func (r *Registry) UnregisterSchema(component, version string) error {
	query := "DELETE FROM cc_schemas WHERE component = ?"
	args := []interface{}{component}

	if version != "" {
		query += " AND version = ?"
		args = append(args, version)
	}

	ctx := context.Background()
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to unregister schema: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("no schemas found to unregister")
	}

	return nil
}

// getRegistryPath returns the registry database path for a workspace
func getRegistryPath(workspace string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	if workspace == "" {
		workspace = "global"
	}

	return filepath.Join(homeDir, ".config", "corpus-callosum", workspace, "registry.db"), nil
}

// DetectWorkspace detects the current workspace from the working directory
func DetectWorkspace() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "global"
	}

	// Check if we're in a workspace directory
	// Pattern: ~/src/ws/{workspace}/*
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "global"
	}

	wsBase := filepath.Join(homeDir, "src", "ws")
	if !strings.HasPrefix(cwd, wsBase) {
		return "global"
	}

	// Extract workspace name
	rel, err := filepath.Rel(wsBase, cwd)
	if err != nil {
		return "global"
	}

	parts := filepath.SplitList(filepath.ToSlash(rel))
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}

	return "global"
}
