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
			content := extractOutputContent(line)
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
