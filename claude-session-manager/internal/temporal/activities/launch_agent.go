package activities

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// LaunchAgentInput contains parameters for launching an agent process
type LaunchAgentInput struct {
	SessionName string            // Name of the session
	SessionID   string            // Unique session identifier (workspace ID)
	WorkDir     string            // Working directory for the agent
	AgentType   string            // Type of agent: "claude" or "gemini"
	Environment map[string]string // Additional environment variables
}

// LaunchAgentOutput contains the result of launching an agent
type LaunchAgentOutput struct {
	PID         int       // Process ID of the launched agent
	SessionID   string    // Session ID for reference
	SessionName string    // Session name for reference
	StartedAt   time.Time // When the process was started
	Command     string    // Command that was executed
}

// LaunchAgentActivity starts a Claude Code or Gemini CLI process
// This is a Temporal activity that handles process spawning and environment setup
func LaunchAgentActivity(ctx context.Context, input LaunchAgentInput) (*LaunchAgentOutput, error) {
	// Validate input
	if input.SessionName == "" {
		return nil, fmt.Errorf("session name cannot be empty")
	}
	if input.WorkDir == "" {
		return nil, fmt.Errorf("working directory cannot be empty")
	}
	if input.AgentType == "" {
		input.AgentType = "claude" // Default to Claude
	}

	// Generate session ID if not provided
	if input.SessionID == "" {
		input.SessionID = uuid.New().String()
	}

	// Validate working directory exists
	if _, err := os.Stat(input.WorkDir); err != nil {
		return nil, fmt.Errorf("working directory does not exist: %w", err)
	}

	// Determine agent command based on type
	var cmdName string
	var cmdArgs []string

	switch input.AgentType {
	case "claude":
		cmdName = "claude"
		cmdArgs = []string{} // Claude Code typically runs without args
	case "gemini":
		cmdName = "gemini-cli"
		cmdArgs = []string{} // Gemini CLI typically runs without args
	default:
		return nil, fmt.Errorf("unsupported agent type: %s", input.AgentType)
	}

	// Build command
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Dir = input.WorkDir

	// Set up environment
	cmd.Env = os.Environ()
	for key, value := range input.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Set session-specific environment variables
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("AGM_SESSION_ID=%s", input.SessionID),
		fmt.Sprintf("AGM_SESSION_NAME=%s", input.SessionName),
	)

	// Set up stdout/stderr capture (pipes for monitoring)
	// Note: In a full implementation, these would be connected to monitoring activities
	// For now, we inherit parent's stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Start the process
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s process: %w", input.AgentType, err)
	}

	// Build output
	output := &LaunchAgentOutput{
		PID:         cmd.Process.Pid,
		SessionID:   input.SessionID,
		SessionName: input.SessionName,
		StartedAt:   startTime,
		Command:     fmt.Sprintf("%s %v", cmdName, cmdArgs),
	}

	// Note: We don't wait for the process here - it runs in the background
	// The workflow will monitor it via MonitorOutputActivity

	return output, nil
}

// GetSessionDataDir returns the data directory for a session
// This helper is used by activities to locate session-specific files
func GetSessionDataDir(sessionID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	sessionDir := filepath.Join(homeDir, ".agm", "sessions", sessionID)
	return sessionDir, nil
}

// EnsureSessionDir creates the session directory if it doesn't exist
func EnsureSessionDir(sessionID string) (string, error) {
	sessionDir, err := GetSessionDataDir(sessionID)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}

	return sessionDir, nil
}
