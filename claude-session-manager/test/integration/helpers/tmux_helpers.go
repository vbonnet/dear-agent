package helpers

import (
	"fmt"
	"os/exec"
	"strings"
)

// HasTmuxSession checks if a tmux session exists
func HasTmuxSession(sessionName string) (bool, error) {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
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
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
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
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
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
	cmd := exec.Command("tmux", "show-options", "-t", sessionName, option)
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
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", workDir)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}
	return nil
}
