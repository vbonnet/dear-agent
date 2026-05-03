package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// InitSequence orchestrates the initialization sequence for a new Claude session
// It uses tmux Control Mode to ensure commands complete before proceeding
type InitSequence struct {
	SessionName    string
	SocketPath     string
	PromptVerified bool // When true, skip redundant WaitForClaudePrompt calls (caller already verified)
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
// This helper consolidates the send pattern used for slash commands,
// which requires: load text into buffer → paste atomically → send Enter.
//
// Bug fix (2026-04-07): Switched from send-keys -l to load-buffer + paste-buffer.
// send-keys -l delivers text character-by-character through the terminal emulator,
// creating a race condition where Enter (C-m) can arrive before the terminal finishes
// processing the text. This caused /rename and /agm:agm-assoc commands during init
// to appear as queued pasted text instead of being submitted.
// paste-buffer is atomic — the entire text appears in the input at once.
func SendCommandLiteral(sessionName, command string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	debug.Log("SendCommandLiteral: Sending command text: %q to session %s", command, sessionName)

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// Lock tmux server for buffer operations (prevent interleaved pastes).
	// Bug fix (2026-04-02): Without this lock, concurrent SendCommandLiteral calls
	// could interleave at the tmux server level, causing cross-session byte leakage.
	return withTmuxLock(func() error {
		// Ensure buffer is cleaned up on any error path.
		bufferLoaded := false
		defer func() {
			if bufferLoaded {
				deleteBuffer()
			}
		}()

		// Step 1: Load command text into tmux paste buffer via stdin.
		// This avoids command-line length limits and special character escaping issues.
		timeout := getAdaptiveTimeout()
		cmdLoad, cancel1 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "load-buffer", "-b", "agm-cmd", "-")
		defer cancel1()

		stdin, err := cmdLoad.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe for load-buffer: %w", err)
		}

		if err := cmdLoad.Start(); err != nil {
			return fmt.Errorf("failed to start load-buffer: %w", err)
		}

		if _, err := stdin.Write([]byte(command)); err != nil {
			stdin.Close()
			cmdLoad.Wait()
			return fmt.Errorf("failed to write to load-buffer stdin: %w", err)
		}
		stdin.Close()

		if err := cmdLoad.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux load-buffer timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to load command into tmux buffer: %w", err)
		}
		bufferLoaded = true
		debug.Log("SendCommandLiteral: Text loaded into buffer successfully")

		// Step 2: Paste buffer to session (atomic operation, -d deletes buffer after paste).
		cmdPaste, cancel2 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "paste-buffer", "-b", "agm-cmd", "-t", normalizedName, "-d")
		defer cancel2()
		if err := cmdPaste.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux paste-buffer timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to paste buffer to tmux session: %w", err)
		}
		bufferLoaded = false // paste-buffer -d already deleted it
		debug.Log("SendCommandLiteral: Buffer pasted successfully")

		// Step 3: Send Enter key to submit the command.
		// Delay prevents tmux from coalescing pasted text with ENTER keystroke.
		// Do not remove.
		time.Sleep(50 * time.Millisecond)
		debug.Log("SendCommandLiteral: Sending C-m (Enter)")

		cmdEnter, cancel3 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "C-m")
		defer cancel3()
		if err := cmdEnter.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux send-keys timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to send Enter: %w", err)
		}
		debug.Log("SendCommandLiteral: Enter sent successfully, command should execute")

		// Step 4: Auto-detect and retry Enter if paste left text unsubmitted.
		// Bug fix (2026-04-10): After paste-buffer, Enter (C-m) sometimes doesn't
		// register. Detect via capture-pane and re-send Enter up to 2 times.
		if err := retryEnterAfterPaste(socketPath, normalizedName, 2); err != nil {
			return err
		}
		debug.Log("SendCommandLiteral: Enter retry check complete")

		return nil
	})
}

// sendRename sends the /rename command and waits for it to complete.
// Uses capture-pane polling (WaitForClaudePrompt) to detect when Claude is ready.
// See ADR-0001 for rationale on capture-pane vs control mode.
func (seq *InitSequence) sendRename() error {
	debug.Log("sendRename: Starting for session %s", seq.SessionName)

	// Wait for Claude prompt before sending /rename.
	// Skip entirely when caller already verified prompt (avoids costly timeout when
	// prompt has scrolled off the 50-line capture buffer).
	if seq.PromptVerified {
		debug.Log("sendRename: Prompt pre-verified by caller, skipping WaitForClaudePrompt")
	} else {
		debug.Log("sendRename: Calling WaitForClaudePrompt with 30s timeout")
		if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
			debug.Log("sendRename: WaitForClaudePrompt FAILED: %v", err)
			return fmt.Errorf("claude not ready for rename: %w", err)
		}
		debug.Log("sendRename: WaitForClaudePrompt succeeded - prompt is visible")
	}

	// Handle trust dialog that may be showing ("Yes, I trust this folder").
	// WaitForClaudePrompt can false-positive on ❯ in the trust dialog's option list.
	// Check if trust dialog is showing by looking for its signature text, then answer it.
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(seq.SessionName)
	checkCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-10")
	if checkOutput, err := checkCmd.CombinedOutput(); err == nil {
		content := string(checkOutput)
		if strings.Contains(content, "Enter to confirm") || strings.Contains(content, "I trust this folder") {
			debug.Log("sendRename: Trust dialog detected, sending Enter to confirm")
			enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "Enter")
			if err := enterCmd.Run(); err != nil {
				debug.Log("sendRename: Failed to confirm trust dialog: %v", err)
			}
			// Wait for Claude to finish loading after trust confirmation
			time.Sleep(3 * time.Second)
			// Re-wait for the actual Claude prompt (❯) after trust is confirmed
			debug.Log("sendRename: Waiting for Claude prompt after trust confirmation")
			if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
				debug.Log("sendRename: WaitForClaudePrompt after trust failed: %v", err)
				return fmt.Errorf("claude not ready after trust confirmation: %w", err)
			}
		}
	}

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
	// Wait for Claude prompt. After /rename completes (with 5s sleep), Claude should
	// be back at the prompt. Skip entirely when pre-verified.
	if seq.PromptVerified {
		debug.Log("sendAssociation: Prompt pre-verified by caller, skipping WaitForClaudePrompt")
	} else {
		if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
			return fmt.Errorf("claude not ready for association: %w", err)
		}
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
