package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Client represents a tmux client for session management
type Client interface {
	// CreateSession creates a new tmux session
	CreateSession(name, workingDir string) error

	// SendKeys sends keys to a tmux session
	SendKeys(sessionName, keys string) error

	// CapturePane captures output from a tmux pane
	CapturePane(sessionName string, lines int) (string, error)

	// KillSession kills a tmux session
	KillSession(sessionName string) error

	// HasSession checks if a tmux session exists
	HasSession(sessionName string) bool

	// WaitForStartup waits for Claude to start in the session
	WaitForStartup(sessionName string, timeout time.Duration) error
}

// New creates a new tmux client
func New() Client {
	return &client{}
}

type client struct{}

func (c *client) CreateSession(name, workingDir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workingDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create tmux session: %w (output: %s)", err, string(output))
	}
	return nil
}

func (c *client) SendKeys(sessionName, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, keys, "C-m")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to send keys to tmux session: %w (output: %s)", err, string(output))
	}
	return nil
}

func (c *client) CapturePane(sessionName string, lines int) (string, error) {
	lineArg := fmt.Sprintf("-%d", lines)
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", lineArg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux pane: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}

func (c *client) KillSession(sessionName string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill tmux session: %w (output: %s)", err, string(output))
	}
	return nil
}

func (c *client) HasSession(sessionName string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	err := cmd.Run()
	return err == nil
}

func (c *client) WaitForStartup(sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			output, err := c.CapturePane(sessionName, 10)
			if err != nil {
				// Session might not be ready yet, continue polling
				if time.Now().After(deadline) {
					return fmt.Errorf("timeout waiting for Claude startup after %v", timeout)
				}
				continue
			}

			// Check if output contains Claude prompt
			if containsClaudePrompt(output) {
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for Claude prompt after %v (last output: %s)", timeout, output)
			}
		}
	}
}

// containsClaudePrompt checks if output contains the Claude prompt
func containsClaudePrompt(output string) bool {
	// Look for Claude Code welcome screen or the input prompt
	// The prompt appears as "> " after horizontal separator lines
	return strings.Contains(output, "Claude Code") ||
		strings.Contains(output, "Welcome back!") ||
		(strings.Contains(output, "───") && strings.Contains(output, "> \n───"))
}
