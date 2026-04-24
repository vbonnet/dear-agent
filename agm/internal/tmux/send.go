package tmux

import (
	"fmt"
	"os"
	"os/exec"
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
//   - agm session new --prompt
//   - agm session send <session> <command>
//   - Any automation that sends commands to Claude
func SendCommandSafe(sessionName string, command string) error {
	// Step 1: Wait for Claude to be ready (detect prompt)
	if err := WaitForPromptSimple(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready: %w\n\nRecovery:\n  1. Check if session exists: agm session list\n  2. Attach to session: agm session attach %s\n  3. Verify Claude is at prompt (look for ❯ marker)", err, sessionName)
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
// Bug fix (2026-03-14): Added shouldInterrupt parameter for conditional ESC
//
// Key behavior:
//   - Waits for Claude prompt before sending
//   - Sends entire file content as ONE command (not line-by-line)
//   - Uses literal mode to prevent special character interpretation
//   - Conditionally sends ESC based on shouldInterrupt flag
//
// Use this for:
//   - agm session new --prompt-file <file>
//   - agm session send <session> --file <file>
func SendPromptFileSafe(sessionName string, filePath string, shouldInterrupt bool) error {
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
	if err := WaitForPromptSimple(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready before sending file: %w", err)
	}

	// Step 4: Send entire file content as one command with conditional interrupt
	if err := SendPromptLiteral(sessionName, string(content), shouldInterrupt); err != nil {
		return fmt.Errorf("failed to send prompt file: %w", err)
	}

	return nil
}

// SendSlashCommandSafe sends a slash command (e.g., /agm:agm-assoc) to Claude.
// This function ensures slash commands execute instead of appearing as text.
//
// Key behavior:
//   - Waits for Claude prompt first (ensures command is recognized)
//   - Sends command with proper timing to avoid queueing
//   - Validates command starts with / (slash commands only)
//
// Use this for:
//   - Sending /agm:agm-assoc, /engram-swarm:start, etc.
//   - Any skill invocation via agm session send
func SendSlashCommandSafe(sessionName string, command string) error {
	// Validate slash command format
	if !strings.HasPrefix(command, "/") {
		return fmt.Errorf("not a slash command (must start with /): %s", command)
	}

	// Wait for Claude to be ready
	if err := WaitForPromptSimple(sessionName, 60*time.Second); err != nil {
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
// Bug fix (2026-03-14): Added shouldInterrupt parameter to control ESC behavior.
// When shouldInterrupt=false, prompts are queued instead of interrupting operations.
//
// Bug fix (2026-03-18): Fixed backwards logic - ALWAYS wait for prompt (like all *Safe functions).
// The shouldInterrupt parameter only controls ESC sending in SendPromptLiteral, not prompt waiting.
//
// Key behavior:
//   - ALWAYS waits for Claude prompt first (consistent with all *Safe functions)
//   - Sends entire text as one literal command (newlines preserved)
//   - Does NOT split on newlines (user wants multi-line input)
//   - Conditionally sends ESC based on shouldInterrupt flag (in SendPromptLiteral)
//
// Use this for:
//   - Prompts with code blocks
//   - Structured prompts with markdown formatting
//   - Any text that needs to preserve newlines
func SendMultiLinePromptSafe(sessionName string, prompt string, shouldInterrupt bool) error {
	// Wait for Claude to be ready (consistent with all other *Safe functions)
	if err := WaitForPromptSimple(sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready for multi-line prompt: %w", err)
	}

	// Bug fix (2026-03-31): Post-submit cooldown — prevent delivering in the same
	// prompt cycle as a human submission. After WaitForPromptSimple returns, the prompt
	// may be transiently visible between human submit and Claude starting to process.
	// Wait, then re-verify the prompt is still there and no human input is present.
	if !shouldInterrupt {
		time.Sleep(1 * time.Second)

		// Re-capture pane to verify prompt stability
		recheck, err := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane",
			"-t", NormalizeTmuxSessionName(sessionName), "-p", "-S", "-5").Output()
		if err == nil {
			recheckContent := string(recheck)
			// If prompt disappeared, session is processing a human submission
			if !containsAnyHarnessPromptPattern(recheckContent) {
				return fmt.Errorf("prompt disappeared after detection — session likely processing human input, aborting delivery")
			}
			// Bug fix (2026-04-10): Skip human-input detection when AI is actively
			// generating (spinner visible). Content changes during generation are
			// AI output, not human typing.
			if !hasActiveSpinner(recheckContent) {
				// If input line has content, human started typing
				if InputLineHasContent(recheckContent) {
					return fmt.Errorf("input line has content after cooldown — human is typing, aborting delivery")
				}
			}
		}
	}

	// Send using literal mode (preserves newlines), with conditional interrupt
	if err := SendPromptLiteral(sessionName, prompt, shouldInterrupt); err != nil {
		return fmt.Errorf("failed to send multi-line prompt: %w", err)
	}

	return nil
}

// SendKeys sends special key names to a session (Down, Up, Tab, Enter, etc.)
// This does NOT use literal mode - it sends the actual key codes to tmux.
//
// Key behavior:
//   - Sends named keys without literal mode
//   - Does NOT append Enter automatically (use "Enter" explicitly if needed)
//   - Useful for navigating UI elements (AskUserQuestion, menus, etc.)
//
// Common key names:
//   - Arrow keys: Up, Down, Left, Right
//   - Special keys: Tab, Enter, Escape, Space
//   - Modifiers: C-c (Ctrl+C), M-x (Alt+X)
//
// Use this for:
//   - Navigating AskUserQuestion option lists
//   - Interacting with CLI menus
//   - Sending control sequences
func SendKeys(sessionName string, keyName string) error {
	socketPath := GetSocketPath()

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// Lock tmux server for send-keys operation.
	// Bug fix (2026-04-02): Without this lock, concurrent send-keys calls could
	// interleave at the tmux server level, causing cross-session byte leakage.
	return withTmuxLock(func() error {
		// Send the key name directly (tmux interprets it)
		// Example: "Down" sends arrow down key, "Tab" sends tab key
		// Note: send-keys targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
		cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, keyName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to send key %s: %w (output: %s)", keyName, err, string(output))
		}

		return nil
	})
}
