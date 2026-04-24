//go:build integration

package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// getTmuxSocket returns the socket path to use for tmux commands
// Respects AGM_TMUX_SOCKET environment variable for test isolation
func getTmuxSocket() string {
	if socket := os.Getenv("AGM_TMUX_SOCKET"); socket != "" {
		return socket
	}
	return "/tmp/agm.sock" // fallback to default
}

// buildTmuxCmd creates a tmux command with the appropriate socket
func buildTmuxCmd(args ...string) *exec.Cmd {
	socket := getTmuxSocket()
	// Prepend -S socket to args
	fullArgs := append([]string{"-S", socket}, args...)
	return exec.Command("tmux", fullArgs...)
}

// BuildTmuxCmd creates a tmux command with the appropriate socket.
// This is an exported version for use in test files.
// It respects AGM_TMUX_SOCKET environment variable for test isolation.
//
// Example usage:
//
//	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName)
//	err := cmd.Run()
func BuildTmuxCmd(args ...string) *exec.Cmd {
	return buildTmuxCmd(args...)
}

// HasTmuxSession checks if a tmux session exists
func HasTmuxSession(sessionName string) (bool, error) {
	cmd := buildTmuxCmd("has-session", "-t", sessionName)
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil // session doesn't exist (exit code 1)
		}
		return false, fmt.Errorf("failed to check tmux session: %w", err)
	}
	return true, nil
}

// KillTmuxSession kills a tmux session
func KillTmuxSession(sessionName string) error {
	cmd := buildTmuxCmd("kill-session", "-t", sessionName)
	err := cmd.Run()
	if err != nil {
		// Ignore error if session doesn't exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("failed to kill tmux session: %w", err)
	}
	return nil
}

// ListTmuxSessions lists tmux sessions matching prefix
func ListTmuxSessions(prefix string) ([]string, error) {
	cmd := buildTmuxCmd("list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No sessions is not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	var sessions []string
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) && trimmed != "" {
			sessions = append(sessions, trimmed)
		}
	}
	return sessions, nil
}

// GetTmuxOption gets a tmux option value for a session
func GetTmuxOption(sessionName, option string) (string, error) {
	cmd := buildTmuxCmd("show-options", "-t", sessionName, option)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get tmux option %s: %w", option, err)
	}

	// Parse output: "option_name value"
	// Example: "aggressive-resize on"
	parts := strings.SplitN(strings.TrimSpace(string(output)), " ", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected tmux option output format: %s", output)
	}
	return strings.TrimSpace(parts[1]), nil
}

// CreateTmuxSession creates a new tmux session (detached)
func CreateTmuxSession(sessionName, workDir string) error {
	cmd := buildTmuxCmd("new-session", "-d", "-s", sessionName, "-c", workDir)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}
	return nil
}

// KillSessionProcesses kills all processes inside a tmux session's panes,
// then kills the session itself. This prevents orphaned processes that survive
// a simple kill-session (which only sends SIGHUP).
func KillSessionProcesses(sessionName string) error {
	// Get pane PIDs via tmux list-panes
	cmd := buildTmuxCmd("list-panes", "-t", sessionName, "-F", "#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		// Session doesn't exist, nothing to kill
		return nil
	}

	for _, line := range strings.Split(string(output), "\n") {
		pidStr := strings.TrimSpace(line)
		if pidStr == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Get process group of the pane process
		pgid, err := syscall.Getpgid(pid)
		if err != nil {
			// Process already exited
			continue
		}

		// Kill entire process group with SIGTERM first
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	}

	// Brief wait for graceful shutdown
	time.Sleep(200 * time.Millisecond)

	// SIGKILL any survivors
	output, _ = buildTmuxCmd("list-panes", "-t", sessionName, "-F", "#{pane_pid}").Output()
	for _, line := range strings.Split(string(output), "\n") {
		pidStr := strings.TrimSpace(line)
		if pidStr == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		pgid, err := syscall.Getpgid(pid)
		if err != nil {
			continue
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}

	// Now kill the tmux session itself
	return KillTmuxSession(sessionName)
}

// KillTmuxServer kills the entire tmux server on the configured socket.
// This terminates ALL sessions and ALL processes on the server.
// Use this for isolated test sockets to ensure complete cleanup.
func KillTmuxServer() error {
	cmd := buildTmuxCmd("kill-server")
	err := cmd.Run()
	if err != nil {
		// Server may not be running
		return nil
	}
	return nil
}
