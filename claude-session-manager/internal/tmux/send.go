package tmux

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// SendCommandSafe sends a command to Claude and waits for execution.
// This is the SAFE version that waits for Claude to be ready before sending.
//
// Key differences from SendCommand:
//  1. Waits for Claude prompt (❯) before sending command
//  2. Detects if session is busy/thinking and returns error
//  3. Better error messages with actionable recovery steps
//
// Use this for:
//  - csm new --prompt
//  - csm send <session> <command>
//  - Any automation that sends commands to Claude
func SendCommandSafe(sessionName string, command string) error {
	// Step 1: Wait for Claude to be ready (detect prompt)
	if err := WaitForClaudePrompt(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready: %w\n\nRecovery:\n  1. Check if session exists: csm list\n  2. Attach to session: csm attach %s\n  3. Verify Claude is at prompt (look for ❯ marker)", err, sessionName)
	}

	// Step 2: Send command using existing SendCommand
	// (SendCommand already handles: literal mode, 100ms delay, Enter key)
	if err := SendCommand(sessionName, command); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	return nil
}

// SendPromptFileSafe sends multi-line prompt from file, waiting for Claude to be ready first.
// This is the SAFE version that waits for prompt before sending each line.
//
// Key behavior:
//  - Waits for Claude prompt before sending
//  - Sends entire file content as ONE command (not line-by-line)
//  - Uses literal mode to prevent special character interpretation
//
// Use this for:
//  - csm new --prompt-file <file>
//  - csm send <session> --file <file>
func SendPromptFileSafe(sessionName string, filePath string) error {
	// Step 1: Validate file exists and read content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read prompt file %s: %w", filePath, err)
	}

	// Step 2: Enforce size limit (10KB)
	const maxPromptFileSize = 10 * 1024
	if len(content) > maxPromptFileSize {
		return fmt.Errorf("prompt file too large: %d bytes (max 10KB)", len(content))
	}

	// Step 3: Wait for Claude to be ready
	if err := WaitForClaudePrompt(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready before sending file: %w", err)
	}

	// Step 4: Send entire file content as one command
	// (Using SendPromptLiteral which handles ESC + literal mode + Enter)
	if err := SendPromptLiteral(sessionName, string(content)); err != nil {
		return fmt.Errorf("failed to send prompt file: %w", err)
	}

	return nil
}

// SendSlashCommandSafe sends a slash command (e.g., /csm-tools:csm-assoc) to Claude.
// This function ensures slash commands execute instead of appearing as text.
//
// Key behavior:
//  - Waits for Claude prompt first (ensures command is recognized)
//  - Sends command with proper timing to avoid queueing
//  - Validates command starts with / (slash commands only)
//
// Use this for:
//  - Sending /csm-tools:csm-assoc, /engram-swarm:start, etc.
//  - Any skill invocation via csm send
func SendSlashCommandSafe(sessionName string, command string) error {
	// Validate slash command format
	if !strings.HasPrefix(command, "/") {
		return fmt.Errorf("not a slash command (must start with /): %s", command)
	}

	// Wait for Claude to be ready
	if err := WaitForClaudePrompt(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready for slash command: %w", err)
	}

	// Send slash command using existing SendCommand
	// (Same as regular command, but we've validated the / prefix)
	if err := SendCommand(sessionName, command); err != nil {
		return fmt.Errorf("failed to send slash command: %w", err)
	}

	return nil
}

// SendMultiLinePromptSafe sends a multi-line prompt as a single command.
// This is for prompts that contain newlines (e.g., code blocks, structured prompts).
//
// Key behavior:
//  - Waits for Claude prompt first
//  - Sends entire text as one literal command (newlines preserved)
//  - Does NOT split on newlines (user wants multi-line input)
//
// Use this for:
//  - Prompts with code blocks
//  - Structured prompts with markdown formatting
//  - Any text that needs to preserve newlines
func SendMultiLinePromptSafe(sessionName string, prompt string) error {
	// Wait for Claude to be ready
	if err := WaitForClaudePrompt(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready for multi-line prompt: %w", err)
	}

	// Send using literal mode (preserves newlines)
	if err := SendPromptLiteral(sessionName, prompt); err != nil {
		return fmt.Errorf("failed to send multi-line prompt: %w", err)
	}

	return nil
}
