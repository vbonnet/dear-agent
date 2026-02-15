package activities

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal/workflows"
)

// StartMCPActivity starts an MCP HTTP server process
// This activity executes the MCP HTTP server with the specified MCP command
func StartMCPActivity(ctx context.Context, input workflows.StartMCPInput) (*workflows.StartMCPResult, error) {
	// Validate input
	if input.Name == "" {
		return nil, fmt.Errorf("MCP service name cannot be empty")
	}
	if input.MCPCommand == "" {
		return nil, fmt.Errorf("MCP command cannot be empty")
	}
	if input.Port == 0 {
		return nil, fmt.Errorf("port cannot be zero")
	}

	// Default server path if not provided
	if input.ServerPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		input.ServerPath = filepath.Join(homeDir, "src/ws/oss/swarm/projects/mcp-global-sharing/packages/mcp-http-server/dist/server.js")
	}

	// Verify server script exists
	if _, err := os.Stat(input.ServerPath); err != nil {
		return nil, fmt.Errorf("MCP HTTP server script not found at %s: %w", input.ServerPath, err)
	}

	// Build command
	// Command: node <server.js> --mcp-command "<mcp-command>" --port <port>
	cmdArgs := []string{
		input.ServerPath,
		"--mcp-command", input.MCPCommand,
		"--port", fmt.Sprintf("%d", input.Port),
	}

	cmd := exec.CommandContext(ctx, "node", cmdArgs...)

	// Set up environment
	cmd.Env = os.Environ()
	for key, value := range input.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Set service-specific environment variables
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("MCP_SERVICE_NAME=%s", input.Name),
		fmt.Sprintf("MCP_HTTP_PORT=%d", input.Port),
	)

	// Ensure MCP data directory exists
	mcpDataDir, err := EnsureMCPDataDir(input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP data directory: %w", err)
	}

	// Set up log files
	logFile := filepath.Join(mcpDataDir, "mcp-server.log")
	errFile := filepath.Join(mcpDataDir, "mcp-server.err")

	// Open log files
	stdout, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout log file: %w", err)
	}
	defer stdout.Close()

	stderr, err := os.OpenFile(errFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr log file: %w", err)
	}
	defer stderr.Close()

	// Connect stdout/stderr to log files
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// Start the process
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP HTTP server: %w", err)
	}

	// Store PID for later use
	pid := cmd.Process.Pid
	pidFile := filepath.Join(mcpDataDir, "mcp-server.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		// Non-fatal error, log but continue
		fmt.Fprintf(os.Stderr, "Warning: failed to write PID file: %v\n", err)
	}

	// Build output
	output := &workflows.StartMCPResult{
		PID:       pid,
		Port:      input.Port,
		StartedAt: startTime,
		Command:   fmt.Sprintf("node %s --mcp-command \"%s\" --port %d", input.ServerPath, input.MCPCommand, input.Port),
	}

	// Wait a moment to ensure process started successfully
	time.Sleep(500 * time.Millisecond)

	// Check if process is still running
	if err := cmd.Process.Signal(os.Signal(nil)); err != nil {
		return nil, fmt.Errorf("MCP server process died immediately after start")
	}

	// Note: We don't wait for the process here - it runs in the background
	// The workflow will monitor it via health checks

	return output, nil
}

// GetMCPDataDir returns the data directory for an MCP service
func GetMCPDataDir(serviceName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	mcpDir := filepath.Join(homeDir, ".agm", "mcp-services", serviceName)
	return mcpDir, nil
}

// EnsureMCPDataDir creates the MCP data directory if it doesn't exist
func EnsureMCPDataDir(serviceName string) (string, error) {
	mcpDir, err := GetMCPDataDir(serviceName)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create MCP data directory: %w", err)
	}

	return mcpDir, nil
}
