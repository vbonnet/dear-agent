package tmux

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const maxPromptFileSize = 10 * 1024 // 10KB

// SendPromptLiteral sends prompt text to tmux session in literal mode
// (-l flag prevents special character interpretation), then sends Enter separately
func SendPromptLiteral(target, prompt string) error {
	// Step 0: Send ESC to interrupt any thinking state
	// This prevents prompts from being queued as "pasted text"
	cmdEsc := exec.Command("tmux", "send-keys", "-t", target, "Escape")
	if err := cmdEsc.Run(); err != nil {
		return fmt.Errorf("failed to send Escape: %w", err)
	}

	// Wait for session to process ESC
	time.Sleep(500 * time.Millisecond)

	// Step 1: Send text in literal mode
	cmd1 := exec.Command("tmux", "send-keys", "-t", target, "-l", prompt)
	if err := cmd1.Run(); err != nil {
		return fmt.Errorf("failed to send prompt text: %w", err)
	}

	// Step 2: Send Enter key separately (as user specified)
	cmd2 := exec.Command("tmux", "send-keys", "-t", target, "C-m")
	if err := cmd2.Run(); err != nil {
		return fmt.Errorf("failed to send Enter key: %w", err)
	}

	// Debug logging if CSM_DEBUG=1
	if os.Getenv("CSM_DEBUG") == "1" {
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

	// Debug logging if CSM_DEBUG=1
	if os.Getenv("CSM_DEBUG") == "1" {
		hash := sha256.Sum256(content)
		fmt.Printf("DEBUG: Sent prompt (hash: %x, length: %d chars, source: --prompt-file %s)\n",
			hash[:8], len(content), filePath)
	}

	// Send using literal mode
	return SendPromptLiteral(target, string(content))
}
