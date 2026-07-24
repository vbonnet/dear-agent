package tmux

import (
	"context"
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
	return SendPromptFileSafeContext(context.Background(), sessionName, filePath, shouldInterrupt)
}

// SendPromptFileSafeContext is the command-scoped file delivery path. Caller
// cancellation stops file preparation and composer polling before prompt bytes
// are written.
func SendPromptFileSafeContext(ctx context.Context, sessionName string, filePath string, shouldInterrupt bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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
	if err := WaitForPromptSimpleContext(ctx, sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready before sending file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
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
	return SendSlashCommandSafeContext(context.Background(), sessionName, command)
}

// SendSlashCommandSafeContext is the command-scoped slash-command path. Caller
// cancellation stops composer polling before the slash command is written.
func SendSlashCommandSafeContext(ctx context.Context, sessionName string, command string) error {
	// Validate slash command format
	if !strings.HasPrefix(command, "/") {
		return fmt.Errorf("not a slash command (must start with /): %s", command)
	}

	// Wait for Claude to be ready
	if err := WaitForPromptSimpleContext(ctx, sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready for slash command: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
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
//
// skipPostSubmitGuard reports whether SendMultiLinePromptSafe should skip its
// post-submit composer-stability check. That check aborts only if the composer
// vanished (session mid-processing); the old human-typing abort here is gone
// (human_typing is non-blocking now — see stash.go). It still yields when the
// caller has a legitimate reason to deliver regardless:
//   - shouldInterrupt: the caller is deliberately interrupting (e.g. work
//     requests / wake-loops);
//   - autonomous: the session is unattended (ce-v9in);
//   - force: the operator passed --force and must follow through (ce-5sow).
//
// Pure decision helper (no tmux I/O) so the gate is unit-testable.
func skipPostSubmitGuard(shouldInterrupt, autonomous, force bool) bool {
	return shouldInterrupt || autonomous || force
}

func SendMultiLinePromptSafe(sessionName string, prompt string, shouldInterrupt bool) error {
	return SendMultiLinePromptSafeContext(context.Background(), sessionName, prompt, shouldInterrupt)
}

// SendMultiLinePromptSafeForHarness preserves the native composer semantics of
// harness while retaining the shared readiness and delivery protocol.
func SendMultiLinePromptSafeForHarness(sessionName string, prompt string, shouldInterrupt bool, harness string) error {
	return SendMultiLinePromptSafeForHarnessContext(context.Background(), sessionName, prompt, shouldInterrupt, harness)
}

// SendMultiLinePromptSafeContext is the command-scoped multiline delivery
// path. Caller cancellation stops readiness polling and the stability delay
// before any prompt bytes are written.
func SendMultiLinePromptSafeContext(ctx context.Context, sessionName string, prompt string, shouldInterrupt bool) error {
	return SendMultiLinePromptSafeForHarnessContext(ctx, sessionName, prompt, shouldInterrupt, "")
}

// SendMultiLinePromptSafeForHarnessContext is the harness-aware command-scoped
// multiline delivery path. In particular, AGY requires raw bracketed paste so
// embedded newlines stay inside one composer submission.
func SendMultiLinePromptSafeForHarnessContext(ctx context.Context, sessionName string, prompt string, shouldInterrupt bool, harness string) error {
	// Wait for the active harness composer (consistent with other *Safe functions).
	if err := WaitForPromptSimpleContext(ctx, sessionName, 60*time.Second); err != nil {
		return fmt.Errorf("session not ready for multi-line prompt: %w", err)
	}

	// Post-submit composer-stability check. After WaitForPromptSimple returns, the
	// prompt may be transiently visible between a human submit and the session
	// starting to process. We pause briefly and re-verify the composer is still
	// present so we do not paste into a vanishing prompt.
	//
	// The human_typing abort that used to live here ("input line has content
	// after cooldown — human is typing") is GONE: it over-captured and blocked
	// the mesh. Leftover input is now stashed non-blockingly in SendPromptLiteral
	// (see stash.go), so we simply proceed. Only the composer-disappeared check
	// remains, and it is a distinct protection (session mid-processing), not
	// human_typing.
	//
	// ce-v9in / ce-5sow: still skipped in autonomous mode, under operator --force,
	// and when shouldInterrupt — those callers deliver regardless.
	if !skipPostSubmitGuard(shouldInterrupt, AutonomousMode(), ForceDelivery()) {
		if err := sleepWithContext(ctx, time.Second); err != nil {
			return err
		}

		// Re-capture pane to verify composer stability
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 5*time.Second)
		recheck, err := exec.CommandContext(cmdCtx, "tmux", "-S", GetSocketPath(), "capture-pane",
			"-t", NormalizeTmuxSessionName(sessionName), "-p", "-e", "-J", "-S", "-30").Output()
		cmdErr := cmdCtx.Err()
		cmdCancel()
		if cmdErr != nil {
			return fmt.Errorf("tmux capture-pane timed out during prompt stability check: %w", cmdErr)
		}
		if err == nil {
			recheckContent := string(recheck)
			// If the composer disappeared, the session is actively processing; abort
			// rather than paste into a vanished prompt. This is NOT human_typing.
			if !containsAnyHarnessPromptPattern(recheckContent) {
				return fmt.Errorf("prompt disappeared after detection — session likely processing, aborting delivery")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Send using literal mode (preserves newlines), with conditional interrupt
	if err := SendPromptLiteralForHarness(sessionName, prompt, shouldInterrupt, harness); err != nil {
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
		// Use raw hex 0x0d for Enter to avoid paste coalescing
		var cmd *exec.Cmd
		if keyName == "Enter" || keyName == "C-m" {
			cmd = exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d")
		} else {
			cmd = exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, keyName)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to send key %s: %w (output: %s)", keyName, err, string(output))
		}

		return nil
	})
}
