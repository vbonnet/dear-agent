package tmux

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxPromptFileSize = 10 * 1024 // 10KB

// hasQueuedInput checks if the session has queued pasted text or user input
func hasQueuedInput(paneContent string) bool {
	// Look for "[Pasted text" pattern which indicates queued input
	if strings.Contains(paneContent, "[Pasted text") {
		return true
	}

	// Look for "Press up to edit queued messages" pattern
	if strings.Contains(paneContent, "Press up to edit queued messages") {
		return true
	}

	return false
}

// SendPromptLiteral sends prompt text to tmux session in literal mode
// (-l flag prevents special character interpretation), then sends Enter separately
// Uses control mode for timing-independent reliable delivery
func SendPromptLiteral(target, prompt string) error {
	socketPath := GetSocketPath()

	// Step 0: Check if there's already text in the input box
	// If user is typing, abort to avoid interfering
	cmdCapture := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", target, "-p")
	output, err := cmdCapture.Output()
	if err != nil {
		return fmt.Errorf("failed to capture pane: %w", err)
	}

	// Check if command line has text (look for "[Pasted text" or other input indicators)
	paneContent := string(output)
	if hasQueuedInput(paneContent) {
		return fmt.Errorf("session has queued input - not sending to avoid interference")
	}

	// Step 1: Send ESC to interrupt any thinking state
	// This prevents prompts from being queued as "pasted text"
	cmdEsc := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target, "Escape")
	if err := cmdEsc.Run(); err != nil {
		return fmt.Errorf("failed to send Escape: %w", err)
	}

	// Wait for session to process ESC
	time.Sleep(500 * time.Millisecond)

	// Step 2: Use control mode to send text + Enter with reliable timing
	// Control mode waits for %end notifications, eliminating race conditions
	ctrl, err := StartControlMode(target)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Send text in literal mode (waits for %end notification)
	cmd1 := fmt.Sprintf("send-keys -t %s -l %q", target, prompt)
	if err := ctrl.SendCommand(cmd1); err != nil {
		return fmt.Errorf("failed to send prompt text: %w", err)
	}

	// Send Enter separately (waits for %end notification)
	// IMPORTANT: C-m must be separate from -l, otherwise it's treated as literal text
	// See: https://github.com/tmux/tmux/issues/1778
	cmd2 := fmt.Sprintf("send-keys -t %s C-m", target)
	if err := ctrl.SendCommand(cmd2); err != nil {
		return fmt.Errorf("failed to send Enter key: %w", err)
	}

	// Debug logging if AGM_DEBUG=1
	if os.Getenv("AGM_DEBUG") == "1" {
		hash := sha256.Sum256([]byte(prompt))
		fmt.Printf("DEBUG: Sent prompt (hash: %x, length: %d chars, source: --prompt)\n",
			hash[:8], len(prompt))
	}

	return nil
}

// SendPromptFromFile reads prompt from file and sends it using literal mode
func SendPromptFromFile(target, filePath string) error {
	// Validate file exists and get size
	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("prompt file not found: %s", filePath)
	}

	// Enforce 10KB size limit
	if stat.Size() > maxPromptFileSize {
		return fmt.Errorf("prompt file too large: %d bytes (max 10KB)", stat.Size())
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read prompt file: %w", err)
	}

	// Debug logging if AGM_DEBUG=1
	if os.Getenv("AGM_DEBUG") == "1" {
		hash := sha256.Sum256(content)
		fmt.Printf("DEBUG: Sent prompt (hash: %x, length: %d chars, source: --prompt-file %s)\n",
			hash[:8], len(content), filePath)
	}

	// Send using literal mode
	return SendPromptLiteral(target, string(content))
}
