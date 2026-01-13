package tmux

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// ClaudePromptPatterns are patterns that indicate Claude is ready for input
var ClaudePromptPatterns = []string{
	"▌",  // Claude cursor
	"> ", // Common prompt
	"$ ", // Shell-style prompt
	"# ", // Root prompt
}

// WaitForClaudePrompt waits for Claude to return to the input prompt
// Uses control mode to monitor output stream and detect prompt patterns
// Handles octal escapes using unescapeOctal from output_watcher.go
func WaitForClaudePrompt(sessionName string, timeout time.Duration) error {
	log.Printf("🔍 Starting prompt detection for session: %s", sessionName)

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
		// Read next output line (short timeout per line)
		line, err := watcher.GetRawLine(1 * time.Second)
		if err != nil {
			// Timeout on individual read - check if we've seen enough idle time
			consecutiveIdleLines++

			// If we've seen a prompt-like pattern and then idle, assume ready
			if consecutiveIdleLines >= 3 && containsPromptPattern(lastContent) {
				log.Printf("✓ Detected prompt pattern after idle period: %q", lastContent)
				return nil
			}

			// If we've checked many lines and seen idle, likely ready
			if linesChecked > 10 && consecutiveIdleLines >= 5 {
				log.Printf("✓ Stable idle state detected after %d lines", linesChecked)
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

			// Log output for debugging (limit verbosity)
			if linesChecked <= 5 || linesChecked%10 == 0 {
				log.Printf("📝 Output [%d]: %q", linesChecked, content)
			}

			// Check for prompt patterns
			if containsPromptPattern(content) {
				log.Printf("✓ Prompt pattern detected in line %d: %q", linesChecked, content)
				// Wait a bit more to ensure it's stable
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}

		// Check for %end notification (command completed)
		if strings.HasPrefix(line, "%end") {
			log.Printf("📋 Command completion detected (%%end) at line %d", linesChecked)
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
	log.Printf("🔍 Waiting for Claude to be ready (session: %s)", sessionName)

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
				log.Printf("✓ Session appears ready (SessionStart hooks completed)")
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
			log.Printf("📝 Output [%d]: %q", linesChecked, truncate(content, 100))
		}

		// Check for trust prompt
		if !trustPromptSeen && strings.Contains(content, "Do you trust the files in this folder?") {
			trustPromptSeen = true
			log.Printf("🛡️  Trust prompt detected at line %d", linesChecked)
		}

		// If trust prompt seen but not answered yet, look for the prompt and answer
		if trustPromptSeen && !trustPromptAnswered {
			// Check if this line contains the selection prompt (❯ 1. Yes, proceed)
			if strings.Contains(content, "Yes, proceed") {
				log.Printf("✓ Answering trust prompt with Enter key")
				trustPromptAnswered = true

				// Close control mode session temporarily to send keys
				ctrl.Close()

				// Use regular tmux send-keys (not via control mode)
				// This works better for interactive prompts
				if err := SendCommand(sessionName, "C-m"); err != nil {
					log.Printf("⚠ Failed to send Enter: %v", err)
					return fmt.Errorf("failed to answer trust prompt: %w", err)
				}

				log.Printf("✓ Trust prompt answer sent, waiting 2s for processing...")
				time.Sleep(2 * time.Second)

				// Restart control mode to continue monitoring
				ctrl, err = StartControlMode(sessionName)
				if err != nil {
					return fmt.Errorf("failed to restart control mode after trust prompt: %w", err)
				}
				// Note: we don't defer close here because it's handled at the function level

				// Recreate watcher for the new control session
				watcher = NewOutputWatcher(ctrl.Stdout)
				log.Printf("✓ Control mode restarted, continuing to monitor...")
			}
		}

		// Check for SessionStart hook completion indicator
		// The hooks write output that ends with "Session Start ===", "success", etc.
		if strings.Contains(content, "=== engram-research Session Start ===") ||
			strings.Contains(content, "SessionStart:startup hook success") ||
			strings.Contains(content, "Hook execution completed") {
			sessionStartSeen = true
			log.Printf("📋 SessionStart hooks activity detected at line %d", linesChecked)
		}

		// Check for Claude prompt (only after trust prompt handled)
		if trustPromptAnswered && sessionStartSeen && containsPromptPattern(content) {
			log.Printf("✓ Claude prompt detected at line %d: %q", linesChecked, truncate(content, 50))
			// Wait a bit to ensure it's stable
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		// Also check for the main prompt pattern even without session start (fallback)
		if !trustPromptSeen && containsPromptPattern(content) {
			log.Printf("✓ Claude prompt detected (no trust prompt) at line %d: %q", linesChecked, truncate(content, 50))
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
