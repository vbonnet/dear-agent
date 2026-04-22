package tmux

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// retryEnterAfterPaste checks if a paste-buffer operation left text unsubmitted
// (Enter didn't register) and automatically re-sends Enter.
//
// Bug fix (2026-04-10): After paste-buffer, Enter (C-m) sometimes doesn't register.
// Text sits in the input buffer unprocessed. This function detects that condition
// by capturing the pane and looking for "[Pasted text" indicators or the pasted
// content still sitting on the input line, then re-sends Enter.
//
// Parameters:
//   - socketPath: tmux socket path
//   - normalizedName: normalized tmux session name (target)
//   - maxRetries: maximum number of Enter retries (typically 2)
//
// Returns nil if Enter was delivered successfully (or was never needed).
// Returns error only if tmux commands fail.
func retryEnterAfterPaste(socketPath, normalizedName string, maxRetries int) error {
	for retry := 0; retry < maxRetries; retry++ {
		// Wait for tmux to process the initial Enter
		delay := 100*time.Millisecond + time.Duration(retry)*200*time.Millisecond
		time.Sleep(delay)

		// Capture recent pane content to check if paste is stuck
		cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-3")
		output, err := cmd.Output()
		if err != nil {
			// Can't capture pane — nothing we can do, but don't fail the send.
			return nil //nolint:nilerr // best-effort: capture failure is non-fatal
		}

		content := string(output)

		// Check if pasted text is sitting unsubmitted
		if !isPasteStuck(content) {
			return nil // Enter was delivered, nothing to retry
		}

		// Paste is stuck — re-send Enter
		if os.Getenv("AGM_DEBUG") == "1" {
			slog.Debug("Enter auto-retry: paste detected but not submitted",
				"retry", retry+1, "maxRetries", maxRetries)
		}

		cmdEnter := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "C-m")
		if err := cmdEnter.Run(); err != nil {
			return fmt.Errorf("failed to re-send Enter on retry %d: %w", retry+1, err)
		}
	}

	return nil
}

// isPasteStuck checks if the pane content indicates a paste-buffer operation
// left text unsubmitted (Enter didn't register).
//
// Detection signals:
//  1. "[Pasted text" indicator — tmux/terminal shows this when paste is queued
//  2. Content on the input line after the prompt character — text pasted but
//     not submitted (only checked when prompt is visible)
func isPasteStuck(paneContent string) bool {
	// Signal 1: Explicit paste indicator
	if strings.Contains(paneContent, "[Pasted text") {
		return true
	}

	// Signal 2: Content sitting on the prompt input line (paste landed but Enter missed)
	// Only meaningful if a prompt character is visible (session is at input)
	if containsAnyHarnessPromptPattern(paneContent) && InputLineHasContent(paneContent) {
		return true
	}

	return false
}
