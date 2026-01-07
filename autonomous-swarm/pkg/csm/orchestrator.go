package csm

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Orchestrator manages CSM session lifecycle
type Orchestrator struct {
	// Optional: CSM binary path (defaults to "csm" in PATH)
	csmBinary string
}

// NewOrchestrator creates a new CSM orchestrator
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		csmBinary: "csm",
	}
}

// NewOrchestratorWithBinary creates an orchestrator with custom CSM binary path
func NewOrchestratorWithBinary(csmBinary string) *Orchestrator {
	return &Orchestrator{
		csmBinary: csmBinary,
	}
}

// Create creates a new CSM session
func (o *Orchestrator) Create(sessionName string) error {
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	cmd := exec.Command(o.csmBinary, "new", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create session %s: %w (output: %s)", sessionName, err, string(output))
	}

	return nil
}

// Monitor checks if a session is healthy
func (o *Orchestrator) Monitor(sessionName string) (bool, error) {
	if sessionName == "" {
		return false, fmt.Errorf("session name cannot be empty")
	}

	// Check CSM session exists
	cmd := exec.Command(o.csmBinary, "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to list CSM sessions: %w", err)
	}

	// Parse JSON output
	var sessions []map[string]interface{}
	if err := json.Unmarshal(output, &sessions); err != nil {
		return false, fmt.Errorf("failed to parse csm list output: %w", err)
	}

	// Check if session exists in list
	sessionExists := false
	for _, session := range sessions {
		if name, ok := session["name"].(string); ok && name == sessionName {
			sessionExists = true
			break
		}
	}

	if !sessionExists {
		return false, nil
	}

	// Check tmux session health
	cmd = exec.Command("tmux", "has-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return false, nil
	}

	return true, nil
}

// Extract retrieves the session UUID
func (o *Orchestrator) Extract(sessionName string) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}

	cmd := exec.Command(o.csmBinary, "get-uuid", sessionName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get UUID for session %s: %w", sessionName, err)
	}

	uuid := strings.TrimSpace(string(output))
	if uuid == "" {
		return "", fmt.Errorf("empty UUID returned for session %s", sessionName)
	}

	return uuid, nil
}

// Archive archives a CSM session
func (o *Orchestrator) Archive(sessionName string) error {
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	cmd := exec.Command(o.csmBinary, "archive", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to archive session %s: %w (output: %s)", sessionName, err, string(output))
	}

	return nil
}

// WaitForSession waits for a session to become available (for testing)
func (o *Orchestrator) WaitForSession(sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		healthy, err := o.Monitor(sessionName)
		if err != nil {
			return fmt.Errorf("failed to monitor session: %w", err)
		}

		if healthy {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for session %s to become available", sessionName)
}
