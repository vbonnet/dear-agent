package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// ClaudePromptPatterns are patterns that indicate Claude is ready for input
var ClaudePromptPatterns = []string{
	"❯",  // Claude Code primary prompt (Unicode U+276F)
	"▌",  // Claude cursor
	"> ", // Common prompt
	"$ ", // Shell-style prompt
	"# ", // Root prompt
}

// WaitForClaudePrompt waits for Claude prompt using capture-pane polling.
// This replaces the control-mode approach which only sees NEW output after attachment.
// capture-pane reads the pane's historical buffer, allowing us to detect prompts
// that appeared before we started monitoring.
//
// If a trust prompt ("Do you trust the files in this folder?") appears during the
// wait, this function auto-answers it by sending Enter (selecting the default
// "Yes, proceed" option) and continues waiting for the Claude prompt. This is
// critical when starting Claude in a sandbox where --add-dir does not pre-trust
// the workspace path.
func WaitForClaudePrompt(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting prompt detection for session: %s (using capture-pane polling)", sessionName)

	// Find which socket the session is on
	socketPath := findSessionSocket(sessionName)

	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond
	checksPerformed := 0
	lastLog := time.Now()
	trustAnswered := false
	var trustAnsweredAt time.Time

	for time.Now().Before(deadline) {
		checksPerformed++

		// Log progress every 10 seconds
		if time.Since(lastLog) > 10*time.Second {
			debug.Log("⏳ Still waiting for prompt... (performed %d checks)", checksPerformed)
			lastLog = time.Now()
		}

		// Capture last 50 lines from pane
		cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p", "-S", "-50")
		output, err := cmd.CombinedOutput()
		if err != nil {
			debug.Log("⚠️  capture-pane failed (attempt %d): %v", checksPerformed, err)
			time.Sleep(pollInterval)
			continue
		}

		content := string(output)

		// Log a sample on first check to verify we're reading output
		if checksPerformed == 1 {
			lines := strings.Split(strings.TrimSpace(content), "\n")
			if len(lines) > 0 {
				lastLine := lines[len(lines)-1]
				debug.Log("📝 Sample output (last line): %q", truncate(lastLine, 100))
			}
		}

		// Check for Claude's specific prompt pattern (❯)
		// Use strict matching to avoid false positives from bash prompts.
		// Suppress detection for ~2s after answering trust to avoid matching
		// the still-visible trust prompt UI ("❯ 1. Yes, proceed").
		if containsClaudePromptPattern(content) {
			if trustAnswered && time.Since(trustAnsweredAt) < 2*time.Second {
				// Trust prompt UI may still be on screen — ignore false match.
			} else {
				debug.Log("✓ Claude prompt detected after %d checks", checksPerformed)
				// Brief sleep to ensure prompt is stable
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}

		// Detect and auto-answer trust prompt inline.
		// Without this, the trust prompt blocks Claude's main UI from rendering,
		// so the ❯ prompt never appears and we time out.
		if !trustAnswered && containsTrustPromptPattern(content) && strings.Contains(content, "Yes, proceed") {
			debug.Log("🛡️  Trust prompt detected — auto-answering with Enter")
			if err := SendKeys(sessionName, "Enter"); err != nil {
				debug.Log("⚠️  Failed to answer trust prompt: %v", err)
				// Don't give up; maybe a retry will succeed.
			} else {
				trustAnswered = true
				trustAnsweredAt = time.Now()
				debug.Log("✓ Trust prompt answered, continuing to wait for ❯")
				// Brief sleep so Claude can transition past the trust UI
				time.Sleep(1 * time.Second)
				continue
			}
		}

		// Sleep before next poll
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for Claude prompt (waited %v, checked %d times)", timeout, checksPerformed)
}

// WaitForClaudePromptControlMode is the old control-mode based implementation
// DEPRECATED: Control mode only sees NEW output after attachment, missing historical output
// Preserved for reference but should not be used for session startup detection
func WaitForClaudePromptControlMode(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting prompt detection for session: %s (control mode - DEPRECATED)", sessionName)

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

	lastLog := time.Now()

	for time.Now().Before(deadline) {
		// Log progress every 10 seconds for debugging hangs
		if time.Since(lastLog) > 10*time.Second {
			debug.Log("⏳ Still waiting for prompt... (checked %d lines, %d consecutive idles)", linesChecked, consecutiveIdleLines)
			lastLog = time.Now()
		}

		// Read next output line (short timeout per line - 200ms for faster detection)
		// Using ReadLine instead of GetRawLine to ensure timeout is enforced via goroutine + select
		line, err := watcher.ReadLine(200 * time.Millisecond)
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

// containsClaudePromptPattern checks if content contains Claude's unique prompt pattern.
// This function is more strict than containsPromptPattern - it ONLY matches
// Claude Code's specific "❯" prompt, not generic shell prompts.
//
// This is used to avoid false positives when bash shell is visible before Claude starts.
// The bash prompt ("$", ">", "#") should NOT be detected as Claude being ready.
func containsClaudePromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}

	// Only check for Claude's specific prompt character (U+276F)
	// This excludes bash prompts like "$", ">", "#"
	return strings.Contains(trimmed, "❯")
}

// containsTrustPromptPattern checks if content contains Claude Code trust prompt.
//
// Claude Code shows a trust prompt when opening untrusted directories:
// "Do you trust the files in this folder?"
//
// This is used during InitSequence to auto-answer trust prompts that appear
// during session creation (e.g., after /rename or /agm:agm-assoc commands).
func containsTrustPromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}

	// Exact text match for trust prompt
	// This text is stable and used consistently by Claude Code
	return strings.Contains(trimmed, "Do you trust the files in this folder?")
}

// containsPromptPattern is deprecated. Use containsClaudePromptPattern instead.
// This function matches bash prompts ("$", ">", "#") which causes false positives
// when bash shell is visible before Claude starts.
// Preserved for backward compatibility with WaitForClaudePrompt (control mode function).
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

// WaitForPromptSimple waits for any supported harness prompt using simple capture-pane approach.
// This is a simplified version that doesn't use control mode (which has issues).
// It periodically captures the pane content and checks for prompt patterns.
// Detects both Claude (❯) and Gemini (">   Type your message") prompts.
func WaitForPromptSimple(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting simple prompt detection for session: %s", sessionName)

	deadline := time.Now().Add(timeout)
	checkCount := 0

	for time.Now().Before(deadline) {
		checkCount++

		// Capture last 5 lines of the pane
		output, err := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-5").Output()
		if err != nil {
			// Session might not exist or not accessible
			time.Sleep(500 * time.Millisecond)
			continue
		}

		lines := strings.Split(string(output), "\n")

		// Check each line for any harness prompt pattern (Claude or Gemini)
		for i, line := range lines {
			if containsAnyHarnessPromptPattern(line) {
				debug.Log("✓ Harness prompt detected in line %d (check #%d): %q", i, checkCount, strings.TrimSpace(line))
				// Found prompt - wait a bit to ensure it's stable
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}

		// Log progress every 10 checks (5 seconds)
		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for prompt... (check #%d)", checkCount)
		}

		// Wait before next check
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for harness prompt (waited %v, performed %d checks)", timeout, checkCount)
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
		line, err := watcher.ReadLine(2 * time.Second)
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

// GeminiPromptPatterns are patterns that indicate Gemini is ready for input
var GeminiPromptPatterns = []string{
	">   Type your message", // Gemini's input prompt text
	"@path/to/file",         // Part of Gemini's input prompt
	"╭─",                    // Box drawing characters from Gemini UI
	"╰─",                    // Box drawing characters from Gemini UI
}

// OpenCodePromptPatterns are patterns that indicate OpenCode is ready for input
var OpenCodePromptPatterns = []string{
	"> ",   // OpenCode input prompt
	"❯",    // OpenCode may use similar prompt to Claude
	">> ",  // Alternative OpenCode prompt pattern
}

// WaitForGeminiPrompt waits for Gemini to return to the input prompt
// Uses control mode to monitor output stream and detect prompt patterns
// Similar to WaitForClaudePrompt but adapted for Gemini's UI patterns
func WaitForGeminiPrompt(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting Gemini prompt detection for session: %s", sessionName)

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
	linesChecked := 0
	promptPatternsSeen := 0

	for time.Now().Before(deadline) {
		// Read next output line (200ms timeout for faster detection)
		line, err := watcher.ReadLine(200 * time.Millisecond)
		if err != nil {
			// Timeout on individual read - check if we've seen enough idle time
			consecutiveIdleLines++

			// If we've seen prompt patterns and then idle, assume ready
			// Increased to 10 consecutive idles (2 seconds) to avoid false positives
			if consecutiveIdleLines >= 10 && promptPatternsSeen >= 2 {
				debug.Log("✓ Detected Gemini prompt after idle period (saw %d patterns)", promptPatternsSeen)
				return nil
			}

			// If we've checked many lines and seen idle, likely ready
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

			// Log output for debugging (limit verbosity)
			if linesChecked <= 5 || linesChecked%10 == 0 {
				if isVisibleContent(content) {
					cleanContent := stripANSI(content)
					if strings.TrimSpace(cleanContent) != "" {
						debug.Log("📝 Output [%d]: %q", linesChecked, truncate(cleanContent, 80))
					}
				}
			}

			// Check for Gemini prompt patterns
			if containsGeminiPromptPattern(content) {
				promptPatternsSeen++
				debug.Log("✓ Gemini prompt pattern detected in line %d: %q (count: %d)", linesChecked, truncate(content, 50), promptPatternsSeen)

				// Need to see multiple patterns to confirm (Gemini's UI has box drawing + text)
				if promptPatternsSeen >= 2 {
					// Wait a bit more to ensure it's stable
					time.Sleep(1 * time.Second)
					return nil
				}
			}
		}

		// Check for %end notification (command completed)
		if strings.HasPrefix(line, "%end") {
			debug.Log("📋 Command completion detected (%%end) at line %d", linesChecked)
		}
	}

	return fmt.Errorf("timeout waiting for Gemini prompt (waited %v, checked %d lines)", timeout, linesChecked)
}

// containsAnyHarnessPromptPattern checks if content contains prompt patterns from
// ANY supported harness (Claude, Gemini, or OpenCode). Used by SendMultiLinePromptSafe and
// SendPromptLiteral which don't know the harness type but need to detect readiness.
func containsAnyHarnessPromptPattern(content string) bool {
	return containsClaudePromptPattern(content) || containsGeminiPromptPattern(content) || containsOpenCodePromptPattern(content)
}

// containsGeminiPromptPattern checks if content contains any Gemini prompt pattern
func containsGeminiPromptPattern(content string) bool {
	// Trim whitespace for comparison
	trimmed := strings.TrimSpace(content)

	// Empty content is not a prompt
	if trimmed == "" {
		return false
	}

	// Check against known Gemini patterns
	for _, pattern := range GeminiPromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	return false
}

// containsOpenCodePromptPattern checks if content contains any OpenCode prompt pattern
func containsOpenCodePromptPattern(content string) bool {
	// Trim whitespace for comparison
	trimmed := strings.TrimSpace(content)

	// Empty content is not a prompt
	if trimmed == "" {
		return false
	}

	// Check against known OpenCode patterns
	for _, pattern := range OpenCodePromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	return false
}

// WaitForGeminiReady waits for Gemini to be fully ready
// This function waits for the Gemini prompt to appear after startup
func WaitForGeminiReady(sessionName string, timeout time.Duration) error {
	debug.Log("🔍 Waiting for Gemini to be ready (session: %s)", sessionName)

	// Start control mode
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := NewOutputWatcher(ctrl.Stdout)

	// State tracking
	deadline := time.Now().Add(timeout)
	linesChecked := 0
	promptPatternsSeen := 0
	bannerSeen := false

	for time.Now().Before(deadline) {
		// Read next output line
		line, err := watcher.ReadLine(2 * time.Second)
		if err != nil {
			// Timeout on individual read - might be ready
			if promptPatternsSeen >= 2 && linesChecked > 10 {
				debug.Log("✓ Gemini appears ready (saw %d prompt patterns)", promptPatternsSeen)
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

		// Check for Gemini ASCII banner (indicates startup)
		if strings.Contains(content, "███") || strings.Contains(content, "GEMINI") {
			if !bannerSeen {
				bannerSeen = true
				debug.Log("🎨 Gemini banner detected at line %d", linesChecked)
			}
		}

		// Check for Gemini prompt patterns
		if containsGeminiPromptPattern(content) {
			promptPatternsSeen++
			debug.Log("✓ Gemini prompt pattern detected at line %d: %q (count: %d)",
				linesChecked, truncate(content, 50), promptPatternsSeen)

			// Need to see multiple patterns to confirm (box drawing + text)
			if promptPatternsSeen >= 2 {
				debug.Log("✓ Gemini prompt fully detected, waiting for stability...")
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}
	}

	return fmt.Errorf("timeout waiting for Gemini to be ready (waited %v, checked %d lines)", timeout, linesChecked)
}

// WaitForOutputIdle detects when tmux pane output has been idle (no new output) for a specified duration.
// This uses capture-pane polling to track output changes over time.
// Returns nil when output has been idle for idleDuration, error on timeout.
//
// This is useful for detecting when a skill or command has finished producing output,
// even if it doesn't print an explicit completion marker.
//
// Example:
//
//	// Wait for output to be idle for 1 second, with 15 second total timeout
//	err := WaitForOutputIdle("my-session", 1*time.Second, 15*time.Second)
func WaitForOutputIdle(sessionName string, idleDuration time.Duration, timeout time.Duration) error {
	debug.Log("🔍 Starting idle detection for session: %s (idle threshold: %v, timeout: %v)",
		sessionName, idleDuration, timeout)

	// Find which socket the session is on
	socketPath := findSessionSocket(sessionName)

	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond // Faster polling for responsive detection

	var lastContent string
	var lastChangeTime time.Time
	checksPerformed := 0

	for time.Now().Before(deadline) {
		checksPerformed++

		// Capture last 50 lines from pane
		cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p", "-S", "-50")
		output, err := cmd.CombinedOutput()
		if err != nil {
			debug.Log("⚠️  capture-pane failed (attempt %d): %v", checksPerformed, err)
			time.Sleep(pollInterval)
			continue
		}

		content := string(output)

		// Initialize on first check
		if checksPerformed == 1 {
			lastContent = content
			lastChangeTime = time.Now()
			debug.Log("📝 Initial output captured (%d bytes)", len(content))
		} else {
			// Check if content has changed
			if content != lastContent {
				// Output changed - reset idle timer
				lastContent = content
				lastChangeTime = time.Now()
				debug.Log("📝 Output changed (check #%d, idle timer reset)", checksPerformed)
			} else {
				// Output unchanged - check idle duration
				idleTime := time.Since(lastChangeTime)
				if idleTime >= idleDuration {
					debug.Log("✓ Output idle detected after %d checks (idle for %v)",
						checksPerformed, idleTime)
					return nil
				}

				// Log progress every 5 checks when close to idle threshold
				if checksPerformed%5 == 0 {
					debug.Log("⏳ Output idle for %v (threshold: %v)", idleTime, idleDuration)
				}
			}
		}

		// Sleep before next poll
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for output idle (waited %v, checked %d times)", timeout, checksPerformed)
}

// WaitForPattern waits for a specific text pattern to appear in tmux pane output.
// This uses capture-pane polling to check for the pattern.
// Returns nil when pattern is found, error on timeout.
//
// This is useful for detecting explicit completion markers or messages from skills/commands.
//
// Example:
//
//	// Wait for skill completion marker
//	err := WaitForPattern("my-session", "[AGM_SKILL_COMPLETE]", 10*time.Second)
func WaitForPattern(sessionName string, pattern string, timeout time.Duration) error {
	debug.Log("🔍 Starting pattern detection for session: %s (pattern: %q, timeout: %v)",
		sessionName, pattern, timeout)

	// Find which socket the session is on
	socketPath := findSessionSocket(sessionName)

	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond
	checksPerformed := 0

	for time.Now().Before(deadline) {
		checksPerformed++

		// Capture last 100 lines from pane (more lines to catch pattern)
		cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p", "-S", "-100")
		output, err := cmd.CombinedOutput()
		if err != nil {
			debug.Log("⚠️  capture-pane failed (attempt %d): %v", checksPerformed, err)
			time.Sleep(pollInterval)
			continue
		}

		content := string(output)

		// Check if pattern exists in output
		if strings.Contains(content, pattern) {
			debug.Log("✓ Pattern found after %d checks: %q", checksPerformed, pattern)
			return nil
		}

		// Log progress every 10 checks
		if checksPerformed%10 == 0 {
			debug.Log("⏳ Still searching for pattern... (check #%d)", checksPerformed)
		}

		// Sleep before next poll
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for pattern %q (waited %v, checked %d times)", pattern, timeout, checksPerformed)
}
