// Package dolt provides Dolt database integration for workspace-scoped storage.
// Each workspace gets an isolated Dolt database with component migration support.
package dolt

import (
	"database/sql"
	"errors"
	"time"
)

// DoltConfig represents configuration for a workspace's Dolt database.
type DoltConfig struct {
	// WorkspaceName is the name of the workspace (e.g., "oss", "acme")
	WorkspaceName string

	// DoltDir is the absolute path to the .dolt directory
	DoltDir string

	// Port is the Dolt server port (e.g., 3307 for oss, 3308 for acme)
	Port int

	// Host is the Dolt server host (default: 127.0.0.1)
	Host string

	// DatabaseName is the database name within Dolt (default: "workspace")
	DatabaseName string

	// AutoStart indicates whether to auto-start Dolt server if not running
	AutoStart bool

	// ServerConfigPath is the path to server.yaml (optional)
	ServerConfigPath string
}

// Migration represents a component migration.
type Migration struct {
	ID              int       `db:"id"`
	Component       string    `db:"component"`
	Version         int       `db:"version"`
	Name            string    `db:"name"`
	Checksum        string    `db:"checksum"`
	AppliedAt       time.Time `db:"applied_at"`
	AppliedBy       string    `db:"applied_by"`
	ExecutionTimeMs int       `db:"execution_time_ms"`
	TablesCreated   string    `db:"tables_created"` // JSON array
}

// ComponentInfo represents a registered component.
type ComponentInfo struct {
	Name        string    `db:"name"`
	Version     string    `db:"version"`
	Prefix      string    `db:"prefix"`
	InstalledAt time.Time `db:"installed_at"`
	UpdatedAt   time.Time `db:"updated_at"`
	Status      string    `db:"status"`
	Manifest    string    `db:"manifest"` // JSON
}

// MigrationFile represents a migration file to be applied.
type MigrationFile struct {
	// Version is the migration version number
	Version int

	// Name is the migration name (e.g., "initial_schema")
	Name string

	// FilePath is the absolute path to the .sql file
	FilePath string

	// SQL is the migration SQL content
	SQL string

	// Checksum is the SHA256 hash of the SQL content
	Checksum string

	// TablesCreated is the list of tables created by this migration
	TablesCreated []string

	// DependsOn lists migration versions this depends on
	DependsOn []int
}

// MigrationResult represents the result of applying a migration.
type MigrationResult struct {
	// Migration is the migration that was applied
	Migration *MigrationFile

	// Success indicates if the migration succeeded
	Success bool

	// ExecutionTimeMs is the time taken to execute the migration
	ExecutionTimeMs int

	// Error is any error that occurred
	Error error

	// DoltCommit is the Dolt commit hash (if committed)
	DoltCommit string
}

// WorkspaceAdapter manages Dolt database for a workspace.
type WorkspaceAdapter struct {
	config *DoltConfig
	db     *sql.DB
}

// Common errors
var (
	ErrDoltNotInstalled     = errors.New("dolt is not installed")
	ErrDoltDirNotFound      = errors.New("dolt directory not found")
	ErrDoltServerNotRunning = errors.New("dolt server is not running")
	ErrDatabaseNotFound     = errors.New("database not found")
	ErrMigrationNotFound    = errors.New("migration not found")
	ErrMigrationFailed      = errors.New("migration failed")
	ErrPrefixCollision      = errors.New("prefix already claimed by another component")
	ErrReservedPrefix       = errors.New("prefix is reserved for system use")
	ErrComponentNotFound    = errors.New("component not found")
	ErrInvalidChecksum      = errors.New("migration checksum does not match")
	ErrDependencyNotMet     = errors.New("migration dependency not satisfied")
	ErrDuplicateMigration   = errors.New("migration already applied")
)

// Reserved prefixes that cannot be used by components
var ReservedPrefixes = []string{
	"dolt_",
	"mysql_",
	"information_schema_",
	"performance_schema_",
	"sys_",
}

// ComponentStatus represents the status of a component.
type ComponentStatus string

const (
	StatusInstalled   ComponentStatus = "installed"
	StatusUninstalled ComponentStatus = "uninstalled"
	StatusError       ComponentStatus = "error"
)

// IsolationTest represents a test for workspace isolation.
type IsolationTest struct {
	WorkspaceA string
	WorkspaceB string
	Verified   bool
	Error      error
}
