package tmux

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxPromptFileSize = 10 * 1024 // 10KB

// QueuedInputType classifies what kind of input is queued in a session
type QueuedInputType int

// QueuedInputType values classifying input observed in a session pane.
const (
	QueuedInputNone  QueuedInputType = iota // No queued input detected
	QueuedInputAGM                          // Queued input is a stuck AGM message ([From: header)
	QueuedInputHuman                        // Queued input is freeform human text
)

// hasActiveSpinner checks if the pane content contains a Claude Code spinner
// character, indicating AI is actively generating output. When the spinner is
// visible, any pane content changes are AI output — not human input.
//
// Bug fix (2026-04-10): Prevents false "human input in progress" detection
// during active AI generation. The spinner characters cycle while Claude is
// thinking/generating, and content changes between captures were being
// misclassified as human typing.
func hasActiveSpinner(paneContent string) bool {
	return strings.ContainsAny(paneContent, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
}

// InputLineHasContent checks if the input line (the line containing the Claude
// prompt character ❯) has any user-typed content after the prompt marker.
// This detects when a human is actively typing and delivery should be aborted.
//
// Bug fix (2026-03-31): Prevents agent messages from interrupting human typing.
// Previously only [Pasted text] indicators were checked; actual typed text on the
// input line was not detected.
func InputLineHasContent(paneContent string) bool {
	lines := strings.Split(paneContent, "\n")
	// Scan from the bottom (most recent) to find the prompt line
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		idx := strings.Index(line, "❯")
		if idx >= 0 {
			// Extract text after the prompt character
			after := line[idx+len("❯"):]
			// Trim leading space (prompt is typically "❯ ") and trailing whitespace
			after = strings.TrimSpace(after)
			return after != ""
		}
	}
	return false
}

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

// ClassifyQueuedInput inspects pane content to determine whether queued input
// is a stuck AGM message or human-typed text. Returns the classification and
// a user-facing error message.
func ClassifyQueuedInput(paneContent string) (QueuedInputType, string) {
	if !hasQueuedInput(paneContent) {
		return QueuedInputNone, ""
	}

	// Look for AGM message header pattern: [From: sender | ID: ... | Sent: ...]
	lines := strings.Split(paneContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[From:") && strings.Contains(trimmed, "| ID:") {
			sender := extractSender(trimmed)
			return QueuedInputAGM, fmt.Sprintf("session has queued AGM message (stuck paste-buffer from %s). Use agm send clear-input SESSION or retry with --force", sender)
		}
	}

	return QueuedInputHuman, "session has human input in progress - not sending. Retry later"
}

// extractSender pulls the sender name from an AGM message header line.
// Format: [From: sender | ID: ... | Sent: ...]
func extractSender(headerLine string) string {
	after, found := strings.CutPrefix(headerLine, "[From: ")
	if !found {
		return "unknown"
	}
	if idx := strings.Index(after, " |"); idx > 0 {
		return after[:idx]
	}
	return "unknown"
}

// SendPromptLiteral sends prompt text to a tmux session using atomic paste-buffer,
// then sends Enter separately.
//
// Bug fix (2026-04-07): Switched from send-keys -l to load-buffer + paste-buffer.
// send-keys -l delivers text character-by-character through the terminal emulator.
// For large messages, the delay before Enter was insufficient — the terminal was
// still rendering when C-m arrived, causing Claude Code to treat the input as
// queued/pasted text ("Press up to edit queued messages") instead of submitting it.
// load-buffer + paste-buffer is atomic and eliminates this race condition entirely.
//
// Bug fix (2026-03-14): Added shouldInterrupt parameter to make ESC sending conditional.
// ESC interrupts Claude's thinking state, which should only happen when explicitly requested.
// When shouldInterrupt=false, prompts are queued instead of interrupting.
func SendPromptLiteral(target, prompt string, shouldInterrupt bool) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedTarget := NormalizeTmuxSessionName(target)

	// Acquire concurrency semaphore to prevent resource exhaustion
	// Bug fix (2026-04-02): SendPromptLiteral previously had no concurrency control,
	// allowing unbounded parallel send-keys operations that could exhaust fds.
	if err := acquireTmuxSemaphore(ctx); err != nil {
		return fmt.Errorf("tmux concurrency limit reached: %w", err)
	}
	defer releaseTmuxSemaphore()

	// Lock tmux server for the entire send sequence (Escape → paste-buffer → C-m → retries).
	// Bug fix (2026-04-02): Without this lock, concurrent SendPromptLiteral calls on different
	// sessions could interleave their multi-step tmux command sequences at the server level,
	// causing stray bytes to leak across sessions and trigger copy-mode on unrelated sessions.
	// The lock serializes all tmux write operations, matching the pattern used by SendCommand.
	return withTmuxLock(func() error {
		// Step 0: Check if there's already text in the input box
		// If user is typing, abort to avoid interfering
		// Note: capture-pane targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
		cmdCapture := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p")
		output, err := cmdCapture.Output()
		if err != nil {
			return fmt.Errorf("failed to capture pane: %w", err)
		}

		// Check if command line has text (look for "[Pasted text" or other input indicators)
		// Bug fix (2026-03-31): shouldInterrupt=true (--force) bypasses ALL input detection.
		// Previously --force only controlled ESC sending but did not bypass hasQueuedInput
		// or InputLineHasContent checks, making it ineffective.
		paneContent := string(output)
		if !shouldInterrupt {
			// Bug fix (2026-04-10): Skip human-input detection when AI is actively
			// generating. The spinner characters (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) cycle during
			// generation, causing pane content changes between captures that were
			// falsely classified as human typing.
			if !hasActiveSpinner(paneContent) {
				if hasQueuedInput(paneContent) {
					_, msg := ClassifyQueuedInput(paneContent)
					return fmt.Errorf("%s", msg)
				}

				// Bug fix (2026-03-31): Check if the input line has typed content.
				// This catches the case where a human is actively typing on the prompt line.
				// Without this check, the agent message would overwrite/append to human's text.
				if InputLineHasContent(paneContent) {
					return fmt.Errorf("input line has content — human is typing, aborting delivery. Retry on next poll cycle")
				}
			}
		}

		// Step 1: Conditionally send ESC to interrupt thinking state (Bug fix: only if shouldInterrupt=true)
		// When shouldInterrupt=false, prompts are queued instead of interrupting active operations.
		// This fixes Bug 2 where ESC was unconditionally sent, interrupting operations.
		if shouldInterrupt {
			// Send ESC to interrupt any thinking state
			// This prevents prompts from being queued as "pasted text"
			cmdEsc := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "Escape")
			if err := cmdEsc.Run(); err != nil {
				return fmt.Errorf("failed to send Escape: %w", err)
			}

			// Wait for session to process ESC
			time.Sleep(500 * time.Millisecond)
		}

		// Step 2: Load prompt text into tmux paste buffer, then paste atomically.
		// Bug fix (2026-04-07): Replaced send-keys -l with load-buffer + paste-buffer.
		// send-keys -l sends text character-by-character, creating a race condition where
		// Enter (C-m) can arrive before the terminal finishes rendering. paste-buffer is
		// atomic — the entire text appears in the input at once, eliminating the race.
		// This matches the reliable pattern used by SendCommand.

		// Ensure buffer is cleaned up on any error path
		bufferLoaded := false
		defer func() {
			if bufferLoaded {
				deleteBuffer()
			}
		}()

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

		if _, err := stdin.Write([]byte(prompt)); err != nil {
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
			return fmt.Errorf("failed to load prompt into tmux buffer: %w", err)
		}
		bufferLoaded = true

		// Paste buffer to session (atomic operation, -d deletes buffer after paste)
		cmdPaste, cancel2 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "paste-buffer", "-b", "agm-cmd", "-t", normalizedTarget, "-d")
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

		// Step 3: Send Enter key to submit the prompt.
		// Delay prevents tmux from coalescing pasted text with ENTER keystroke.
		time.Sleep(50 * time.Millisecond)

		cmd2 := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "C-m")
		if err := cmd2.Run(); err != nil {
			return fmt.Errorf("failed to send Enter key: %w", err)
		}

		// Step 3.5: Fast-path Enter retry — detect and fix the common case where
		// Enter didn't register immediately after paste-buffer.
		// Bug fix (2026-04-10): After paste-buffer, C-m sometimes doesn't register.
		// This quick retry (100-300ms) handles the common case before falling through
		// to the longer Step 4 verification loop.
		if err := retryEnterAfterPaste(socketPath, normalizedTarget, 2); err != nil {
			return err
		}

		// Step 4: Post-send verification — handle edge case where session started
		// inference between prompt detection and text delivery.
		// Bug fix (2026-03-29): agm send msg pastes into input buffer instead of submitting
		for retry := 0; retry < 5; retry++ {
			time.Sleep(500 * time.Millisecond)

			cmdCheck := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p", "-S", "-5")
			checkOutput, err := cmdCheck.Output()
			if err != nil {
				break
			}

			checkContent := string(checkOutput)
			if !hasQueuedInput(checkContent) {
				break // Message was submitted normally
			}

			// Text is queued — only re-send Enter if the prompt is visible
			// (session has returned from inference and is ready for input)
			if containsAnyHarnessPromptPattern(checkContent) {
				if os.Getenv("AGM_DEBUG") == "1" {
					slog.Debug("Detected queued [Pasted text] at prompt — re-sending Enter", "retry", retry+1)
				}
				cmdResubmit := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "C-m")
				_ = cmdResubmit.Run()
				// Brief pause then re-check to confirm submission
				time.Sleep(300 * time.Millisecond)
				cmdVerify := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p", "-S", "-5")
				verifyOutput, err := cmdVerify.Output()
				if err == nil && !hasQueuedInput(string(verifyOutput)) {
					break // Successfully submitted
				}
			}
			// Session still working — loop will wait and retry
		}

		if os.Getenv("AGM_DEBUG") == "1" {
			hash := sha256.Sum256([]byte(prompt))
			slog.Debug("Sent prompt", "hash", fmt.Sprintf("%x", hash[:8]), "length", len(prompt), "source", "--prompt")
		}

		return nil
	})
}

// SendPromptFromFile reads prompt from file and sends it using literal mode
// Bug fix (2026-03-14): Added shouldInterrupt parameter
func SendPromptFromFile(target, filePath string, shouldInterrupt bool) error {
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

	if os.Getenv("AGM_DEBUG") == "1" {
		hash := sha256.Sum256(content)
		slog.Debug("Sent prompt", "hash", fmt.Sprintf("%x", hash[:8]), "length", len(content), "source", "--prompt-file "+filePath)
	}

	// Send using literal mode with conditional interrupt
	return SendPromptLiteral(target, string(content), shouldInterrupt)
}
