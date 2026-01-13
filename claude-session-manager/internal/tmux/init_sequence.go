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

// Run executes the initialization sequence via tmux control mode:
// 1. Prime: Send /rename to generate UUID (waits for confirmation)
// 2. Associate: Send /csm-tools:csm-assoc
// Note: Caller is responsible for waiting for ready-file signal after this completes.
func (seq *InitSequence) Run() error {
	// Step 1: Start control mode
	ctrl, err := StartControlMode(seq.SessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher to monitor control mode stream
	watcher := NewOutputWatcher(ctrl.Stdout)

	// Step 2: Prime the session with /rename
	if err := seq.sendRename(ctrl, watcher); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	// Step 3: Associate the session
	if err := seq.sendAssociation(ctrl, watcher); err != nil {
		return fmt.Errorf("association failed: %w", err)
	}

	return nil
}

// sendRename sends the /rename command and waits for it to complete
func (seq *InitSequence) sendRename(ctrl *ControlModeSession, watcher *OutputWatcher) error {
	renameCmd := fmt.Sprintf("/rename %s", seq.SessionName)

	// Send command using send-keys in control mode
	// Use -l flag for literal text, then send Enter separately
	sendLiteralCmd := fmt.Sprintf("send-keys -t %s -l %q", seq.SessionName, renameCmd)
	if _, err := fmt.Fprintf(ctrl.Stdin, "%s\n", sendLiteralCmd); err != nil {
		return fmt.Errorf("failed to send rename text: %w", err)
	}

	// Send Enter to execute the command
	sendEnterCmd := fmt.Sprintf("send-keys -t %s C-m", seq.SessionName)
	if _, err := fmt.Fprintf(ctrl.Stdin, "%s\n", sendEnterCmd); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	// /rename completes silently (no output), so just wait for the prompt to return
	// The prompt indicator (❯) appears when Claude is ready for the next command
	if err := watcher.WaitForPattern("❯", 10*time.Second); err != nil {
		return fmt.Errorf("rename did not complete (prompt not returned): %w", err)
	}

	return nil
}

// sendAssociation sends /csm-tools:csm-assoc command
// Note: This only sends the command via control mode. The caller is responsible
// for waiting for the ready-file signal if needed (to allow for custom progress reporting).
func (seq *InitSequence) sendAssociation(ctrl *ControlModeSession, watcher *OutputWatcher) error {
	assocCmd := fmt.Sprintf("/csm-tools:csm-assoc %s", seq.SessionName)

	// Send command using send-keys in control mode
	// Use -l flag for literal text, then send Enter separately
	sendLiteralCmd := fmt.Sprintf("send-keys -t %s -l %q", seq.SessionName, assocCmd)
	if _, err := fmt.Fprintf(ctrl.Stdin, "%s\n", sendLiteralCmd); err != nil {
		return fmt.Errorf("failed to send association text: %w", err)
	}

	// Send Enter to execute the command
	sendEnterCmd := fmt.Sprintf("send-keys -t %s C-m", seq.SessionName)
	if _, err := fmt.Fprintf(ctrl.Stdin, "%s\n", sendEnterCmd); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	// Command sent successfully - ready-file wait is handled by caller
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
	csmDir := filepath.Join(homeDir, ".csm")
	return filepath.Join(csmDir, fmt.Sprintf("ready-%s", sessionName))
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
