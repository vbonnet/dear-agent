package dolt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// StartDoltServer starts the Dolt SQL server for the workspace.
func (a *WorkspaceAdapter) StartDoltServer(ctx context.Context) error {
	// Check if Dolt is installed
	if !IsDoltInstalled() {
		return ErrDoltNotInstalled
	}

	// Check if .dolt directory exists
	if _, err := os.Stat(a.config.DoltDir); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrDoltDirNotFound, a.config.DoltDir)
	}

	// Check if server is already running
	running, err := a.IsDoltServerRunning(ctx)
	if err == nil && running {
		return nil // Already running
	}

	// Start server
	serverConfigPath := a.config.ServerConfigPath
	if serverConfigPath == "" {
		serverConfigPath = filepath.Join(a.config.DoltDir, "server.yaml")
	}

	var cmd *exec.Cmd
	if _, err := os.Stat(serverConfigPath); err == nil {
		// Start with config file
		cmd = exec.CommandContext(ctx, "dolt", "sql-server", "--config", serverConfigPath)
	} else {
		// Start with command-line args
		cmd = exec.CommandContext(ctx, "dolt", "sql-server",
			"--host", a.config.Host,
			"--port", fmt.Sprintf("%d", a.config.Port),
			"--user", "root",
		)
	}

	cmd.Dir = a.config.DoltDir
	cmd.Stdout = nil // Don't capture output
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start dolt server: %w", err)
	}

	// Wait for server to be ready (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for dolt server to start")
		case <-ticker.C:
			running, err := a.IsDoltServerRunning(ctx)
			if err == nil && running {
				return nil
			}
		}
	}
}

// StopDoltServer stops the Dolt SQL server (if running).
func (a *WorkspaceAdapter) StopDoltServer(ctx context.Context) error {
	// This is tricky - we need to find the process and kill it
	// For now, we'll just try to connect and send SHUTDOWN
	if a.db != nil {
		_, err := a.db.ExecContext(ctx, "SHUTDOWN")
		return err
	}
	return fmt.Errorf("not connected to database")
}

// IsDoltServerRunning checks if the Dolt server is running on the configured port.
func (a *WorkspaceAdapter) IsDoltServerRunning(ctx context.Context) (bool, error) {
	// Try to ping the database
	tempAdapter := &WorkspaceAdapter{config: a.config}
	if err := tempAdapter.Connect(ctx); err != nil {
		return false, nil
	}
	defer tempAdapter.Close()

	return true, nil
}

// InitializeDoltDatabase initializes a new Dolt database in the workspace.
func InitializeDoltDatabase(workspaceRoot string) error {
	doltDir := filepath.Join(workspaceRoot, ".dolt")

	// Check if already initialized
	if _, err := os.Stat(doltDir); err == nil {
		return nil // Already initialized
	}

	// Check if Dolt is installed
	if !IsDoltInstalled() {
		return ErrDoltNotInstalled
	}

	// Create .dolt directory
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		return fmt.Errorf("failed to create .dolt directory: %w", err)
	}

	// Initialize Dolt repository
	cmd := exec.Command("dolt", "init")
	cmd.Dir = doltDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to initialize dolt: %w\noutput: %s", err, output)
	}

	// Create workspace database
	cmd = exec.Command("dolt", "sql", "-q", "CREATE DATABASE IF NOT EXISTS workspace")
	cmd.Dir = doltDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create workspace database: %w\noutput: %s", err, output)
	}

	// Initial commit
	cmd = exec.Command("dolt", "add", ".")
	cmd.Dir = doltDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage changes: %w\noutput: %s", err, output)
	}

	cmd = exec.Command("dolt", "commit", "-m", "Initialize workspace database")
	cmd.Dir = doltDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit: %w\noutput: %s", err, output)
	}

	return nil
}

// CreateServerConfig creates a server.yaml configuration file for the workspace.
func CreateServerConfig(workspaceName string, workspaceRoot string, port int) error {
	doltDir := filepath.Join(workspaceRoot, ".dolt")
	serverConfigPath := filepath.Join(doltDir, "server.yaml")

	// Check if already exists
	if _, err := os.Stat(serverConfigPath); err == nil {
		return nil // Already exists
	}

	config := fmt.Sprintf(`log_level: info

user:
  name: root
  password: ""

listener:
  host: 127.0.0.1
  port: %d
  max_connections: 100

databases:
  - name: workspace
    path: %s

behavior:
  autocommit: true
  read_only: false

performance:
  query_parallelism: 4
`, port, filepath.Join(doltDir, "dolt-db"))

	if err := os.WriteFile(serverConfigPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write server.yaml: %w", err)
	}

	return nil
}

// CommitChanges commits changes to the Dolt database.
func (a *WorkspaceAdapter) CommitChanges(ctx context.Context, message string) error {
	if a.config.DoltDir == "" {
		return fmt.Errorf("dolt directory not configured")
	}

	// Stage changes
	cmd := exec.CommandContext(ctx, "dolt", "add", ".")
	cmd.Dir = a.config.DoltDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage changes: %w\noutput: %s", err, output)
	}

	// Commit
	cmd = exec.CommandContext(ctx, "dolt", "commit", "-m", message)
	cmd.Dir = a.config.DoltDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit: %w\noutput: %s", err, output)
	}

	return nil
}

// GetDoltStatus returns the current Dolt status.
func (a *WorkspaceAdapter) GetDoltStatus(ctx context.Context) (string, error) {
	if a.config.DoltDir == "" {
		return "", fmt.Errorf("dolt directory not configured")
	}

	cmd := exec.CommandContext(ctx, "dolt", "status")
	cmd.Dir = a.config.DoltDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get dolt status: %w", err)
	}

	return string(output), nil
}

// GetDoltLog returns recent Dolt commits.
func (a *WorkspaceAdapter) GetDoltLog(ctx context.Context, limit int) (string, error) {
	if a.config.DoltDir == "" {
		return "", fmt.Errorf("dolt directory not configured")
	}

	cmd := exec.CommandContext(ctx, "dolt", "log", "-n", fmt.Sprintf("%d", limit))
	cmd.Dir = a.config.DoltDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get dolt log: %w", err)
	}

	return string(output), nil
}
