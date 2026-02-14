package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/debug"
)

// InitSequence orchestrates the initialization sequence for a new Claude session
// It uses tmux Control Mode to ensure commands complete before proceeding
type InitSequence struct {
	SessionName string
	SocketPath  string
}

// NewInitSequence creates a new initialization sequencer
func NewInitSequence(sessionName string) *InitSequence {
	return &InitSequence{
		SessionName: sessionName,
		SocketPath:  GetSocketPath(),
	}
}

// Run executes the initialization sequence using capture-pane polling:
// 1. Prime: Send /rename to generate UUID (waits for confirmation)
// 2. Associate: Send /agm:agm-assoc
// Note: Caller is responsible for waiting for ready-file signal after this completes.
//
// Uses WaitForClaudePrompt (capture-pane polling) instead of control mode for prompt detection.
// See ADR-0001 for rationale on why capture-pane is preferred over control mode.
//
// Note: Does NOT acquire tmux lock here because SendCommand (called by SendCommandLiteral)
// already handles locking. Attempting to lock here causes double-lock errors.
func (seq *InitSequence) Run() error {
	// Step 1: Prime the session with /rename
	if err := seq.sendRename(); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	// Step 2: Associate the session
	if err := seq.sendAssociation(); err != nil {
		return fmt.Errorf("association failed: %w", err)
	}

	return nil
}

// SendCommandLiteral sends a command as literal text to a tmux session.
// This helper consolidates the send-keys pattern used for slash commands,
// which requires: send literal text → sleep 100ms → send Enter.
// The 100ms delay ensures tmux receives the text before Enter (see tmux/tmux#1778).
func SendCommandLiteral(sessionName, command string) error {
	socketPath := GetSocketPath()

	debug.Log("SendCommandLiteral: Sending command text: %q to session %s", command, sessionName)

	// Send command text using send-keys with -l flag (literal)
	// Use exec.Command directly instead of SendCommand (which uses load-buffer)
	cmdText := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", command)
	if err := cmdText.Run(); err != nil {
		debug.Log("SendCommandLiteral: FAILED to send text: %v", err)
		return fmt.Errorf("failed to send command text: %w", err)
	}
	debug.Log("SendCommandLiteral: Text sent successfully")

	// Longer delay to ensure text is received and processed before sending Enter
	// See: https://github.com/tmux/tmux/issues/1778
	// Increased to 500ms to prevent command queueing issues in detached sessions
	time.Sleep(500 * time.Millisecond)
	debug.Log("SendCommandLiteral: Sending C-m (Enter)")

	// Send Enter to execute the command
	cmdEnter := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m")
	if err := cmdEnter.Run(); err != nil {
		debug.Log("SendCommandLiteral: FAILED to send Enter: %v", err)
		return fmt.Errorf("failed to send Enter: %w", err)
	}
	debug.Log("SendCommandLiteral: Enter sent successfully, command should execute")

	return nil
}

// sendRename sends the /rename command and waits for it to complete.
// Uses capture-pane polling (WaitForClaudePrompt) to detect when Claude is ready.
// See ADR-0001 for rationale on capture-pane vs control mode.
func (seq *InitSequence) sendRename() error {
	debug.Log("sendRename: Starting for session %s", seq.SessionName)

	// Wait for Claude to be ready BEFORE sending command
	// This ensures we don't send /rename to bash shell (which would fail)
	debug.Log("sendRename: Calling WaitForClaudePrompt with 30s timeout")
	if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
		debug.Log("sendRename: WaitForClaudePrompt FAILED: %v", err)
		return fmt.Errorf("Claude not ready for rename: %w", err)
	}
	debug.Log("sendRename: WaitForClaudePrompt succeeded")

	renameCmd := fmt.Sprintf("/rename %s", seq.SessionName)

	// Send command using helper (consolidates send-keys + Enter logic)
	if err := SendCommandLiteral(seq.SessionName, renameCmd); err != nil {
		return fmt.Errorf("failed to send rename command: %w", err)
	}
	debug.Log("sendRename: /rename command sent, waiting 5s for execution")

	// Wait longer for /rename to fully complete and Claude to be ready
	// 5 seconds gives Claude time to process the rename and return to prompt
	time.Sleep(5 * time.Second)
	debug.Log("sendRename: Wait complete, /rename should be done")

	return nil
}

// sendAssociation sends /agm:agm-assoc command.
// Uses capture-pane polling (WaitForClaudePrompt) to detect when Claude is ready.
// Note: Caller is responsible for waiting for ready-file signal to confirm association completed.
// See ADR-0001 for rationale on capture-pane vs control mode.
func (seq *InitSequence) sendAssociation() error {
	// Wait for Claude to be ready BEFORE sending command
	// This ensures we don't send /agm:agm-assoc to bash shell (which would fail)
	if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
		return fmt.Errorf("Claude not ready for association: %w", err)
	}

	assocCmd := fmt.Sprintf("/agm:agm-assoc %s", seq.SessionName)

	// Send command using helper (consolidates send-keys + Enter logic)
	if err := SendCommandLiteral(seq.SessionName, assocCmd); err != nil {
		return fmt.Errorf("failed to send association command: %w", err)
	}

	// Command sent successfully - ready-file wait is handled by caller
	// (Association completion takes longer, so ready-file is the definitive signal)
	return nil
}

// waitForReadyFile waits for the ready-file signal to appear
// This indicates that the association process has completed
func (seq *InitSequence) waitForReadyFile(timeout time.Duration) error {
	readyPath := getReadyFilePath(seq.SessionName)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if ready file exists
		if _, err := os.Stat(readyPath); err == nil {
			// File exists - association complete!
			return nil
		}

		// Wait a bit before checking again
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for ready file: %s (waited %v)", readyPath, timeout)
}

// getReadyFilePath returns the path to the ready-file for a session
func getReadyFilePath(sessionName string) string {
	homeDir, _ := os.UserHomeDir()
	agmDir := filepath.Join(homeDir, ".agm")
	return filepath.Join(agmDir, fmt.Sprintf("ready-%s", sessionName))
}

// CleanupReadyFile removes the ready-file if it exists
// This should be called before starting a new session with the same name
func CleanupReadyFile(sessionName string) error {
	readyPath := getReadyFilePath(sessionName)
	if err := os.Remove(readyPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already removed
		}
		return fmt.Errorf("failed to remove ready file: %w", err)
	}
	return nil
}

// WaitForReadyFileWithProgress waits for ready-file with progress reporting
// This is a public version that can be called from new.go
func WaitForReadyFileWithProgress(sessionName string, timeout time.Duration, progressFunc func(elapsed time.Duration)) error {
	readyPath := getReadyFilePath(sessionName)
	deadline := time.Now().Add(timeout)
	startTime := time.Now()

	for time.Now().Before(deadline) {
		// Check if ready file exists
		if _, err := os.Stat(readyPath); err == nil {
			// File exists - ready!
			return nil
		}

		// Report progress if callback provided
		if progressFunc != nil {
			elapsed := time.Since(startTime)
			progressFunc(elapsed)
		}

		// Wait before checking again
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for ready file after %v", timeout)
}
