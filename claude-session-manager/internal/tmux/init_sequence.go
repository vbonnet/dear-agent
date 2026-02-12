package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Run executes the initialization sequence via tmux control mode:
// 1. Prime: Send /rename to generate UUID (waits for confirmation)
// 2. Associate: Send /agm:agm-assoc
// Note: Caller is responsible for waiting for ready-file signal after this completes.
func (seq *InitSequence) Run() error {
	// Lock tmux server for init sequence (prevent parallel command sends)
	return withTmuxLock(func() error {
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
	})
}

// sendRename sends the /rename command and waits for it to complete
func (seq *InitSequence) sendRename(ctrl *ControlModeSession, watcher *OutputWatcher) error {
	// Wait for Claude to be ready BEFORE sending command
	// This ensures we don't send /rename to bash shell (which would fail)
	if err := seq.waitForClaudePrompt(ctrl, watcher, 30*time.Second); err != nil {
		return fmt.Errorf("Claude not ready for rename: %w", err)
	}

	renameCmd := fmt.Sprintf("/rename %s", seq.SessionName)

	// Send command text using send-keys with -l flag (literal)
	// This preserves the C-m/Enter fix from commit 76d3053
	sendLiteralCmd := fmt.Sprintf("send-keys -t %s -l %q", seq.SessionName, renameCmd)
	if err := ctrl.SendCommand(sendLiteralCmd); err != nil {
		return fmt.Errorf("failed to send rename text: %w", err)
	}

	// Small delay to ensure text is received before sending Enter
	// See: https://github.com/tmux/tmux/issues/1778
	time.Sleep(100 * time.Millisecond)

	// Send Enter to execute the command
	sendEnterCmd := fmt.Sprintf("send-keys -t %s C-m", seq.SessionName)
	if err := ctrl.SendCommand(sendEnterCmd); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	// Wait for command to complete AFTER sending (Claude returns to prompt)
	// /rename completes quickly, so shorter timeout (10s) is acceptable
	if err := seq.waitForClaudePrompt(ctrl, watcher, 10*time.Second); err != nil {
		return fmt.Errorf("rename command timeout: %w", err)
	}

	return nil
}

// sendAssociation sends /agm:agm-assoc command
// Note: This sends the command and waits for Claude to be ready, but the caller is responsible
// for waiting for the ready-file signal to confirm association completed (for custom progress reporting).
func (seq *InitSequence) sendAssociation(ctrl *ControlModeSession, watcher *OutputWatcher) error {
	// Wait for Claude to be ready BEFORE sending command
	// This ensures we don't send /agm:agm-assoc to bash shell (which would fail)
	if err := seq.waitForClaudePrompt(ctrl, watcher, 30*time.Second); err != nil {
		return fmt.Errorf("Claude not ready for association: %w", err)
	}

	assocCmd := fmt.Sprintf("/agm:agm-assoc %s", seq.SessionName)

	// Send command text using send-keys with -l flag (literal)
	// This preserves the C-m/Enter fix from commit 76d3053
	sendLiteralCmd := fmt.Sprintf("send-keys -t %s -l %q", seq.SessionName, assocCmd)
	if err := ctrl.SendCommand(sendLiteralCmd); err != nil {
		return fmt.Errorf("failed to send association text: %w", err)
	}

	// Small delay to ensure text is received before sending Enter
	// See: https://github.com/tmux/tmux/issues/1778
	time.Sleep(100 * time.Millisecond)

	// Send Enter to execute the command
	sendEnterCmd := fmt.Sprintf("send-keys -t %s C-m", seq.SessionName)
	if err := ctrl.SendCommand(sendEnterCmd); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	// Command sent successfully - ready-file wait is handled by caller
	// (Association completion takes longer, so ready-file is the definitive signal)
	return nil
}

// waitForClaudePrompt waits for Claude to become ready by polling the output stream.
// This is used internally by InitSequence to ensure Claude is ready before
// sending slash commands.
//
// It monitors the control mode output stream for Claude's unique prompt pattern ("❯")
// and returns once detected, or times out if Claude never becomes ready.
//
// This function also handles trust prompts that may appear during session creation.
// When a trust prompt is detected, it automatically answers with "Yes, proceed" and
// continues waiting for the Claude prompt.
func (seq *InitSequence) waitForClaudePrompt(ctrl *ControlModeSession, watcher *OutputWatcher, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Trust prompt state tracking
	trustPromptDetected := false
	trustAnsweringDeadline := time.Time{} // Zero value = no trust answering in progress

	for {
		select {
		case <-ticker.C:
			// Get latest output from watcher (last 5 lines should be enough to detect prompt)
			lines := watcher.GetRecentOutput(5)

			// Check each line for prompts
			for _, line := range lines {
				// Check for Claude ready prompt (original logic)
				if containsClaudePromptPattern(line) {
					// Claude prompt detected - ready for commands
					return nil
				}

				// Check for trust prompt (NEW)
				if !trustPromptDetected && containsTrustPromptPattern(line) {
					trustPromptDetected = true
					trustAnsweringDeadline = time.Now().Add(10 * time.Second)
					debug.Log("Trust prompt detected during InitSequence")
					// Continue to answering logic below
				}

				// If trust detected, look for "Yes, proceed" option (NEW)
				if trustPromptDetected && strings.Contains(line, "Yes, proceed") {
					debug.Log("Answering trust prompt with Enter")

					// Close control mode before sending keys
					// (mixing control mode + send-keys doesn't work well)
					ctrl.Close()

					// Send Enter key to select "Yes, proceed"
					if err := SendCommand(seq.SessionName, "C-m"); err != nil {
						return fmt.Errorf("failed to answer trust prompt: %w", err)
					}

					// Recreate control mode session and watcher
					var err error
					ctrl, err = StartControlMode(seq.SessionName)
					if err != nil {
						return fmt.Errorf("failed to restart control mode after trust: %w", err)
					}
					watcher = NewOutputWatcher(ctrl.Stdout)

					debug.Log("Trust answered, continuing to wait for Claude prompt")

					// Reset trust detection (answered)
					trustPromptDetected = false
					trustAnsweringDeadline = time.Time{}

					// Continue polling for Claude "❯" prompt
					continue
				}
			}

			// Check trust answering timeout (NEW)
			if !trustAnsweringDeadline.IsZero() && time.Now().After(trustAnsweringDeadline) {
				return fmt.Errorf("timeout waiting for trust prompt 'Yes, proceed' option")
			}

			// Check main timeout
			if time.Now().After(deadline) {
				if trustPromptDetected {
					return fmt.Errorf("trust prompt detected but couldn't answer (waited %v)", timeout)
				}
				return fmt.Errorf("timeout waiting for Claude prompt (waited %v)", timeout)
			}
		}
	}
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
