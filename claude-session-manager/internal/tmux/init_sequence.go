package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
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
func (seq *InitSequence) Run() error {
	// Lock tmux server for init sequence (prevent parallel command sends)
	return withTmuxLock(func() error {
		// Step 1: Prime the session with /rename
		if err := seq.sendRename(); err != nil {
			return fmt.Errorf("rename failed: %w", err)
		}

		// Step 2: Associate the session
		if err := seq.sendAssociation(); err != nil {
			return fmt.Errorf("association failed: %w", err)
		}

		return nil
	})
}

// SendCommandLiteral sends a command as literal text to a tmux session.
// This helper consolidates the send-keys pattern used for slash commands,
// which requires: send literal text → sleep 100ms → send Enter.
// The 100ms delay ensures tmux receives the text before Enter (see tmux/tmux#1778).
func SendCommandLiteral(sessionName, command string) error {
	// Send command text using send-keys with -l flag (literal)
	// SendCommand takes: sessionName, command (where command is the full send-keys arguments)
	sendKeysCmd := fmt.Sprintf("-l %q", command)
	if err := SendCommand(sessionName, sendKeysCmd); err != nil {
		return fmt.Errorf("failed to send command text: %w", err)
	}

	// Small delay to ensure text is received before sending Enter
	// See: https://github.com/tmux/tmux/issues/1778
	time.Sleep(100 * time.Millisecond)

	// Send Enter to execute the command
	if err := SendCommand(sessionName, "C-m"); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	return nil
}

// sendRename sends the /rename command and waits for it to complete.
// Uses capture-pane polling (WaitForClaudePrompt) to detect when Claude is ready.
// See ADR-0001 for rationale on capture-pane vs control mode.
func (seq *InitSequence) sendRename() error {
	// Wait for Claude to be ready BEFORE sending command
	// This ensures we don't send /rename to bash shell (which would fail)
	if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
		return fmt.Errorf("Claude not ready for rename: %w", err)
	}

	renameCmd := fmt.Sprintf("/rename %s", seq.SessionName)

	// Send command using helper (consolidates send-keys + Enter logic)
	if err := SendCommandLiteral(seq.SessionName, renameCmd); err != nil {
		return fmt.Errorf("failed to send rename command: %w", err)
	}

	// Note: We don't wait after /rename completes because capture-pane is stateless.
	// The ready-file signal (waited for by caller) confirms the full sequence completed.
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
