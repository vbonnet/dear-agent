package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver for Dolt
)

// NewWorkspaceAdapter creates a new Dolt workspace adapter.
func NewWorkspaceAdapter(config *DoltConfig) (*WorkspaceAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate config
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	adapter := &WorkspaceAdapter{
		config: config,
	}

	return adapter, nil
}

// Connect establishes a connection to the Dolt database.
func (a *WorkspaceAdapter) Connect(ctx context.Context) error {
	if a.db != nil {
		return nil // Already connected
	}

	// Build connection string
	connStr := fmt.Sprintf("root@tcp(%s:%d)/%s",
		a.config.Host,
		a.config.Port,
		a.config.DatabaseName,
	)

	// Open connection
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		if a.config.AutoStart {
			// Try to start Dolt server
			if err := a.StartDoltServer(ctx); err != nil {
				return fmt.Errorf("dolt server not running and auto-start failed: %w", err)
			}
			// Retry connection
			db, err = sql.Open("mysql", connStr)
			if err != nil {
				return fmt.Errorf("failed to open database connection after server start: %w", err)
			}
			if err := db.PingContext(ctx); err != nil {
				db.Close()
				return fmt.Errorf("%w: %v", ErrDoltServerNotRunning, err)
			}
		} else {
			return fmt.Errorf("%w: %v", ErrDoltServerNotRunning, err)
		}
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	a.db = db
	return nil
}

// Close closes the database connection.
func (a *WorkspaceAdapter) Close() error {
	if a.db != nil {
		err := a.db.Close()
		a.db = nil
		return err
	}
	return nil
}

// DB returns the underlying database connection.
func (a *WorkspaceAdapter) DB() *sql.DB {
	return a.db
}

// Config returns the adapter configuration.
func (a *WorkspaceAdapter) Config() *DoltConfig {
	return a.config
}

// InitializeDatabase creates the migration and component registry tables.
func (a *WorkspaceAdapter) InitializeDatabase(ctx context.Context) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}

	// Create migration registry table
	migrationTableSQL := `
CREATE TABLE IF NOT EXISTS dolt_migrations (
  id INT AUTO_INCREMENT PRIMARY KEY,
  component VARCHAR(255) NOT NULL,
  version INT NOT NULL,
  name VARCHAR(255) NOT NULL,
  checksum VARCHAR(64) NOT NULL,
  applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  applied_by VARCHAR(255),
  execution_time_ms INT,
  tables_created TEXT,
  UNIQUE KEY (component, version),
  INDEX idx_component (component),
  INDEX idx_applied_at (applied_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Dolt migration registry - tracks applied component migrations';
`

	if _, err := a.db.ExecContext(ctx, migrationTableSQL); err != nil {
		return fmt.Errorf("failed to create dolt_migrations table: %w", err)
	}

	// Create component registry table
	componentTableSQL := `
CREATE TABLE IF NOT EXISTS component_registry (
  name VARCHAR(255) PRIMARY KEY,
  version VARCHAR(50) NOT NULL,
  prefix VARCHAR(50) NOT NULL UNIQUE,
  installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  status ENUM('installed', 'uninstalled', 'error') DEFAULT 'installed',
  manifest JSON,
  INDEX idx_status (status),
  INDEX idx_prefix (prefix)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Component registry - tracks installed components and prefixes';
`

	if _, err := a.db.ExecContext(ctx, componentTableSQL); err != nil {
		return fmt.Errorf("failed to create component_registry table: %w", err)
	}

	return nil
}

// Ping tests the database connection.
func (a *WorkspaceAdapter) Ping(ctx context.Context) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}
	return a.db.PingContext(ctx)
}

// validateConfig validates the DoltConfig.
func validateConfig(config *DoltConfig) error {
	if config.WorkspaceName == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}

	if config.DoltDir == "" {
		return fmt.Errorf("dolt directory cannot be empty")
	}

	// Check if DoltDir is absolute
	if !filepath.IsAbs(config.DoltDir) {
		return fmt.Errorf("dolt directory must be absolute path: %s", config.DoltDir)
	}

	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("invalid port: %d", config.Port)
	}

	if config.Host == "" {
		config.Host = "127.0.0.1"
	}

	if config.DatabaseName == "" {
		config.DatabaseName = "workspace"
	}

	return nil
}

// ExecuteInTransaction executes a function within a database transaction.
func (a *WorkspaceAdapter) ExecuteInTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if a.db == nil {
		return fmt.Errorf("not connected to database")
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// IsDoltInstalled checks if Dolt is installed on the system.
func IsDoltInstalled() bool {
	_, err := os.Stat("/usr/local/bin/dolt")
	if err == nil {
		return true
	}
	// Try PATH
	_, err = exec.LookPath("dolt")
	return err == nil
}

// GetDefaultConfig creates a default DoltConfig for a workspace.
func GetDefaultConfig(workspaceName string, workspaceRoot string) *DoltConfig {
	// Determine port based on workspace name
	port := 3307 // Default for "oss"
	if workspaceName == "acme" {
		port = 3308
	} else if workspaceName != "oss" {
		// For other workspaces, use a hash-based port
		port = 3309 + hashWorkspaceName(workspaceName)%100
	}

	return &DoltConfig{
		WorkspaceName: workspaceName,
		DoltDir:       filepath.Join(workspaceRoot, ".dolt"),
		Port:          port,
		Host:          "127.0.0.1",
		DatabaseName:  "workspace",
		AutoStart:     false,
	}
}

// hashWorkspaceName creates a simple hash of workspace name for port allocation.
func hashWorkspaceName(name string) int {
	hash := 0
	for _, c := range name {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}
