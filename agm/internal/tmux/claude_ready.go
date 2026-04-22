package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// ClaudeReadyFile manages the ready-file signal created by SessionStart hooks.
// This provides a deterministic way to detect when Claude CLI is ready to accept
// commands, replacing fragile text-parsing-based prompt detection.
//
// Flow:
//  1. agm creates pending marker: ~/.agm/pending-{session-name}
//  2. agm starts Claude with AGM_SESSION_NAME env var
//  3. SessionStart hook (user-configured) creates: ~/.agm/claude-ready-{session-name}
//  4. agm detects ready-file and proceeds with initialization
type ClaudeReadyFile struct {
	sessionName string
}

// NewClaudeReadyFile creates a new ready-file manager for a session
func NewClaudeReadyFile(sessionName string) *ClaudeReadyFile {
	return &ClaudeReadyFile{sessionName: sessionName}
}

// PendingPath returns the path to the pending marker file
func (r *ClaudeReadyFile) PendingPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".agm", fmt.Sprintf("pending-%s", r.sessionName))
}

// ReadyPath returns the path to the ready signal file (created by SessionStart hook)
func (r *ClaudeReadyFile) ReadyPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".agm", fmt.Sprintf("claude-ready-%s", r.sessionName))
}

// CreatePending creates the pending marker file.
// This signals to the SessionStart hook that agm is waiting for a ready signal.
func (r *ClaudeReadyFile) CreatePending() error {
	homeDir, _ := os.UserHomeDir()
	agmDir := filepath.Join(homeDir, ".agm")
	if err := os.MkdirAll(agmDir, 0755); err != nil {
		return fmt.Errorf("failed to create .agm directory: %w", err)
	}

	f, err := os.Create(r.PendingPath())
	if err != nil {
		return fmt.Errorf("failed to create pending file: %w", err)
	}
	f.Close()
	return nil
}

// WaitForReady waits for the SessionStart hook to create the ready-file.
// Returns an error if the timeout is reached before the file appears.
func (r *ClaudeReadyFile) WaitForReady(timeout time.Duration, progressFunc func(elapsed time.Duration)) error {
	deadline := time.Now().Add(timeout)
	startTime := time.Now()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		// Check if ready file exists
		if _, err := os.Stat(r.ReadyPath()); err == nil {
			return nil
		}

		// Report progress if callback provided
		if progressFunc != nil {
			elapsed := time.Since(startTime)
			progressFunc(elapsed)
		}

		<-ticker.C
	}

	return fmt.Errorf("timeout waiting for Claude ready signal after %v (hook may not be configured)", timeout)
}

// Cleanup removes both pending and ready files.
// Call this before starting a new session to ensure clean state.
func (r *ClaudeReadyFile) Cleanup() error {
	// Remove both files, ignoring errors if they don't exist
	os.Remove(r.PendingPath())
	os.Remove(r.ReadyPath())
	return nil
}

// Exists checks if the ready-file exists (for testing/debugging)
func (r *ClaudeReadyFile) Exists() bool {
	_, err := os.Stat(r.ReadyPath())
	return err == nil
}

// PendingExists checks if the pending file exists (for testing/debugging)
func (r *ClaudeReadyFile) PendingExists() bool {
	_, err := os.Stat(r.PendingPath())
	return err == nil
}

// TriggerHookManually manually runs the SessionStart hook since hooks don't run
// when Claude is started non-interactively via tmux send-keys.
// This is a workaround for the limitation that SessionStart hooks only run for
// interactive Claude sessions.
func (r *ClaudeReadyFile) TriggerHookManually() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	hookPath := filepath.Join(homeDir, ".claude", "hooks", "session-start", "agm-ready-signal")

	// Check if hook exists
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		debug.Log("Hook not found at %s, skipping manual trigger", hookPath)
		return nil // Not an error - hook may not be installed
	}

	// Run the hook with AGM_SESSION_NAME env var
	cmd := exec.Command(hookPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("AGM_SESSION_NAME=%s", r.sessionName))

	debug.Log("Manually triggering SessionStart hook: %s", hookPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debug.Log("Hook execution failed: %v (output: %s)", err, string(output))
		return fmt.Errorf("failed to run hook: %w", err)
	}

	debug.Log("Hook triggered successfully (output: %s)", string(output))
	return nil
}
