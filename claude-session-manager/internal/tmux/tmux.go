package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HasSession checks if tmux session exists
func HasSession(name string) (bool, error) {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means session doesn't exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check tmux session: %w", err)
	}
	return true, nil
}

// NewSession creates a new tmux session
func NewSession(name string, workDir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}
	return nil
}

// AttachSession attaches to tmux session or switches if already inside tmux
// Returns nil if session exists and was successfully switched/attached
// In non-interactive environments (no TTY), it skips attach and returns nil
func AttachSession(name string) error {
	// Check if we're already inside a tmux session
	if os.Getenv("TMUX") != "" {
		// Already in tmux - use switch-client instead of attach
		cmd := exec.Command("tmux", "switch-client", "-t", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to switch to tmux session: %w", err)
		}
		return nil
	}

	// Not in tmux - check if we have a TTY available
	// If stdin is not a terminal (e.g., running from Claude Code), skip attach
	// The session is still ready, just can't interactively attach
	if fileInfo, _ := os.Stdin.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		// No TTY available - session exists and commands were sent, just can't attach
		return nil
	}

	// Have a TTY - use attach-session
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

// SendCommand sends a command to tmux pane
func SendCommand(sessionName string, command string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, command, "C-m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send command to tmux: %w", err)
	}
	return nil
}

// Version returns tmux version
func Version() (string, error) {
	cmd := exec.Command("tmux", "-V")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get tmux version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ListSessions returns all active tmux session names
func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// If no tmux server running, return empty list
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}
