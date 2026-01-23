package tmux

import (
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/debug"
)

// ClaudePromptPatterns are patterns that indicate Claude is ready for input
var ClaudePromptPatterns = []string{
	"❯",  // Claude Code primary prompt (Unicode U+276F)
	"▌",  // Claude cursor
	"> ", // Common prompt
	"$ ", // Shell-style prompt
	"# ", // Root prompt
}

// WaitForClaudePrompt waits for Claude to return to the input prompt
// Uses control mode to monitor output stream and detect prompt patterns
// Handles octal escapes using unescapeOctal from output_watcher.go
func WaitForClaudePrompt(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting prompt detection for session: %s", sessionName)

	// Start control mode
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := NewOutputWatcher(ctrl.Stdout)

	// Wait for prompt pattern
	deadline := time.Now().Add(timeout)
	consecutiveIdleLines := 0
	lastContent := ""
	linesChecked := 0

	for time.Now().Before(deadline) {
		// Read next output line (short timeout per line - 200ms for faster detection)
		line, err := watcher.GetRawLine(200 * time.Millisecond)
		if err != nil {
			// Timeout on individual read - check if we've seen enough idle time
			consecutiveIdleLines++

			// If we've seen a prompt-like pattern and then idle, assume ready
			// Increased to 10 consecutive idles (2 seconds) to avoid false detection
			// during slash command execution where output might contain ">" characters
			if consecutiveIdleLines >= 10 && containsPromptPattern(lastContent) {
				debug.Log("✓ Detected prompt pattern after idle period: %q", lastContent)
				return nil
			}

			// If we've checked many lines and seen idle, likely ready
			// Increased to 15 consecutive idles (3 seconds) for more conservative detection
			if linesChecked > 10 && consecutiveIdleLines >= 15 {
				debug.Log("✓ Stable idle state detected after %d lines", linesChecked)
				return nil
			}

			continue
		}

		// Reset idle counter
		consecutiveIdleLines = 0
		linesChecked++

		// Extract content if it's an %output line
		if strings.HasPrefix(line, "%output") {
			content := ExtractOutputContent(line)
			lastContent = content

			// Log output for debugging (limit verbosity and filter escape sequences)
			if linesChecked <= 5 || linesChecked%10 == 0 {
				// Only log if content is meaningful (not just escape sequences)
				if isVisibleContent(content) {
					// Strip ANSI escape sequences before logging
					cleanContent := stripANSI(content)
					if strings.TrimSpace(cleanContent) != "" {
						debug.Log("📝 Output [%d]: %q", linesChecked, truncate(cleanContent, 80))
					}
				}
			}

			// Check for prompt patterns
			if containsPromptPattern(content) {
				debug.Log("✓ Prompt pattern detected in line %d: %q", linesChecked, content)
				// Wait a bit more to ensure it's stable (increased to 2s to avoid false positives)
				time.Sleep(2 * time.Second)
				return nil
			}
		}

		// Check for %end notification (command completed)
		if strings.HasPrefix(line, "%end") {
			debug.Log("📋 Command completion detected (%%end) at line %d", linesChecked)
			// Command finished, likely ready for input soon
			// Continue monitoring to confirm
		}
	}

	return fmt.Errorf("timeout waiting for Claude prompt (waited %v, checked %d lines)", timeout, linesChecked)
}

// containsPromptPattern checks if content contains any prompt pattern
func containsPromptPattern(content string) bool {
	// Trim whitespace for comparison
	trimmed := strings.TrimSpace(content)

	// Empty content is not a prompt
	if trimmed == "" {
		return false
	}

	// Check against known patterns
	for _, pattern := range ClaudePromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	// Check if ends with common prompt characters
	if strings.HasSuffix(trimmed, ">") ||
		strings.HasSuffix(trimmed, "$") ||
		strings.HasSuffix(trimmed, "#") {
		return true
	}

	return false
}

// WaitForClaudeReady waits for Claude to be fully ready, handling trust prompts if needed
// This function:
// 1. Detects and auto-answers trust prompts ("Yes, proceed")
// 2. Waits for SessionStart hooks to complete
// 3. Waits for the Claude prompt (❯) to appear
func WaitForClaudeReady(sessionName string, timeout time.Duration) error {
	debug.Log("🔍 Waiting for Claude to be ready (session: %s)", sessionName)

	// Start control mode
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := NewOutputWatcher(ctrl.Stdout)

	// State tracking
	trustPromptSeen := false
	trustPromptAnswered := false
	deadline := time.Now().Add(timeout)
	linesChecked := 0

	sessionStartSeen := false

	for time.Now().Before(deadline) {
		// Read next output line
		line, err := watcher.GetRawLine(2 * time.Second)
		if err != nil {
			// Timeout on individual read - might be ready
			// Only consider it ready if we've seen SessionStart hooks complete
			if sessionStartSeen && linesChecked > 20 {
				debug.Log("✓ Session appears ready (SessionStart hooks completed)")
				return nil
			}
			continue
		}

		linesChecked++

		// Extract content if it's an %output line
		var content string
		if strings.HasPrefix(line, "%output") {
			content = ExtractOutputContent(line)
		} else {
			content = line
		}

		// Log output for debugging (first few lines and periodically)
		if linesChecked <= 10 || linesChecked%20 == 0 {
			if isVisibleContent(content) {
				cleanContent := stripANSI(content)
				if strings.TrimSpace(cleanContent) != "" {
					debug.Log("📝 Output [%d]: %q", linesChecked, truncate(cleanContent, 100))
				}
			}
		}

		// Check for trust prompt
		if !trustPromptSeen && strings.Contains(content, "Do you trust the files in this folder?") {
			trustPromptSeen = true
			debug.Log("🛡️  Trust prompt detected at line %d", linesChecked)
		}

		// If trust prompt seen but not answered yet, look for the prompt and answer
		if trustPromptSeen && !trustPromptAnswered {
			// Check if this line contains the selection prompt (❯ 1. Yes, proceed)
			if strings.Contains(content, "Yes, proceed") {
				debug.Log("✓ Answering trust prompt with Enter key")
				trustPromptAnswered = true

				// Close control mode session temporarily to send keys
				ctrl.Close()

				// Use regular tmux send-keys (not via control mode)
				// This works better for interactive prompts
				if err := SendCommand(sessionName, "C-m"); err != nil {
					debug.Log("⚠ Failed to send Enter: %v", err)
					return fmt.Errorf("failed to answer trust prompt: %w", err)
				}

				debug.Log("✓ Trust prompt answer sent, waiting 2s for processing...")
				time.Sleep(2 * time.Second)

				// Restart control mode to continue monitoring
				ctrl, err = StartControlMode(sessionName)
				if err != nil {
					return fmt.Errorf("failed to restart control mode after trust prompt: %w", err)
				}
				// Note: we don't defer close here because it's handled at the function level

				// Recreate watcher for the new control session
				watcher = NewOutputWatcher(ctrl.Stdout)
				debug.Log("✓ Control mode restarted, continuing to monitor...")
			}
		}

		// Check for SessionStart hook completion indicator
		// The hooks write output that ends with "Session Start ===", "success", etc.
		if strings.Contains(content, "=== engram-research Session Start ===") ||
			strings.Contains(content, "SessionStart:startup hook success") ||
			strings.Contains(content, "Hook execution completed") {
			sessionStartSeen = true
			debug.Log("📋 SessionStart hooks activity detected at line %d", linesChecked)
		}

		// Check for Claude prompt (only after trust prompt handled)
		if trustPromptAnswered && sessionStartSeen && containsPromptPattern(content) {
			debug.Log("✓ Claude prompt detected at line %d: %q", linesChecked, truncate(content, 50))
			// Wait a bit to ensure it's stable
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		// Also check for the main prompt pattern even without session start (fallback)
		if !trustPromptSeen && containsPromptPattern(content) {
			debug.Log("✓ Claude prompt detected (no trust prompt) at line %d: %q", linesChecked, truncate(content, 50))
			time.Sleep(500 * time.Millisecond)
			return nil
		}
	}

	return fmt.Errorf("timeout waiting for Claude to be ready (waited %v, checked %d lines)", timeout, linesChecked)
}

// truncate truncates a string to maxLen characters with "..." suffix
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// isVisibleContent returns true if content contains visible characters
// (not just ANSI escape sequences)
func isVisibleContent(s string) bool {
	// Empty or whitespace-only strings are not visible
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}

	// If content is mostly escape sequences, don't consider it visible
	// Escape sequences typically start with \x1b or \033
	if strings.HasPrefix(trimmed, "\x1b") || strings.HasPrefix(trimmed, "\033") {
		// Check if there's any non-escape content
		// Simple heuristic: if more than 50% is escape codes, skip it
		escapeCount := strings.Count(trimmed, "\x1b") + strings.Count(trimmed, "\033")
		if escapeCount*4 > len(trimmed) { // Escape sequences are typically 4+ chars
			return false
		}
	}

	return true
}

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	// Remove all ANSI escape sequences
	// Pattern: ESC [ ... m (color codes)
	//          ESC ] ... (OSC sequences)
	//          ESC ? ... (private modes like bracketed paste)
	result := s

	// Remove CSI sequences (ESC [ ... letter)
	for {
		start := strings.Index(result, "\x1b[")
		if start == -1 {
			break
		}
		// Find the end of the sequence (a letter A-Z, a-z)
		end := start + 2
		for end < len(result) {
			ch := result[end]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				end++
				break
			}
			end++
		}
		result = result[:start] + result[end:]
	}

	// Remove OSC sequences (ESC ] ... BEL/ST)
	for {
		start := strings.Index(result, "\x1b]")
		if start == -1 {
			break
		}
		// Find BEL (0x07) or ST (ESC \)
		end := strings.IndexAny(result[start:], "\x07")
		if end == -1 {
			stIdx := strings.Index(result[start:], "\x1b\\")
			if stIdx == -1 {
				break
			}
			end = stIdx + 2
		} else {
			end++
		}
		result = result[:start] + result[start+end:]
	}

	// Remove bracketed paste mode sequences (ESC ? ... h/l)
	for {
		start := strings.Index(result, "\x1b?")
		if start == -1 {
			break
		}
		end := start + 2
		for end < len(result) && result[end] != 'h' && result[end] != 'l' {
			end++
		}
		if end < len(result) {
			end++
		}
		result = result[:start] + result[end:]
	}

	return result
}
