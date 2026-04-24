package tmux

import (
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// PromptDeliveryResult describes the outcome of a prompt delivery verification.
type PromptDeliveryResult struct {
	Delivered bool   // true if prompt was verified as delivered
	Attempt   int    // which attempt succeeded (1-indexed), 0 if all failed
	Method    string // how delivery was confirmed ("processing", "keyword_match", "content_echo")
}

// VerifyPromptDelivery checks that a prompt was actually delivered to the session
// by capturing the pane and looking for evidence of delivery. If the prompt appears
// stuck (idle prompt visible, no processing), it retries sending up to maxRetries times
// with exponential backoff.
//
// Verification signals (any one confirms delivery):
//  1. Session is processing: spinner visible, no idle prompt
//  2. Keyword match: one or more keywords from the prompt appear in scrollback
//  3. Prompt gone: the harness prompt character disappeared (session accepted input)
//
// Parameters:
//   - sessionName: tmux session to verify
//   - promptText: the original prompt text (used to extract keywords for matching)
//   - sendFunc: function to re-send the prompt (called on retry)
//   - maxRetries: maximum number of re-send attempts (typically 3)
//
// Returns the delivery result and any error. A non-nil error indicates a tmux
// failure, not a delivery failure — check result.Delivered for delivery status.
func VerifyPromptDelivery(sessionName, promptText string, sendFunc func() error, maxRetries int) (PromptDeliveryResult, error) {
	keywords := extractKeywords(promptText)
	debug.Log("Verifying prompt delivery (keywords: %v, maxRetries: %d)", keywords, maxRetries)

	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		// Wait for the session to process the prompt before checking.
		// First attempt uses shorter wait (prompt was just sent); retries use backoff.
		var waitDuration time.Duration
		if attempt == 1 {
			waitDuration = 2 * time.Second
		} else {
			// Exponential backoff: 2s, 4s, 8s for retries
			waitDuration = time.Duration(1<<uint(attempt)) * time.Second
		}
		debug.Log("Verify attempt %d/%d: waiting %v before capture-pane check", attempt, maxRetries+1, waitDuration)
		time.Sleep(waitDuration)

		// Capture pane content to check delivery status
		content, err := CapturePaneOutput(sessionName, 50)
		if err != nil {
			return PromptDeliveryResult{}, fmt.Errorf("capture-pane failed during delivery verification: %w", err)
		}

		// Check 1: Is the session processing? (spinner visible = prompt was accepted)
		if hasActiveSpinner(content) {
			debug.Log("✓ Delivery verified (attempt %d): session is processing (spinner active)", attempt)
			return PromptDeliveryResult{Delivered: true, Attempt: attempt, Method: "processing"}, nil
		}

		// Check 2: Do keywords from the prompt appear in the scrollback?
		// Claude Code echoes user messages in the pane, so if we see our keywords,
		// the prompt was delivered.
		if len(keywords) > 0 && keywordsFoundInContent(keywords, content) {
			debug.Log("✓ Delivery verified (attempt %d): keywords found in pane content", attempt)
			return PromptDeliveryResult{Delivered: true, Attempt: attempt, Method: "keyword_match"}, nil
		}

		// Check 3: Has the prompt character disappeared? (session accepted input)
		// If no harness prompt is visible, the session is processing something.
		if !containsAnyHarnessPromptPattern(content) {
			debug.Log("✓ Delivery verified (attempt %d): prompt character gone (session processing)", attempt)
			return PromptDeliveryResult{Delivered: true, Attempt: attempt, Method: "content_echo"}, nil
		}

		// Delivery not confirmed — prompt might be stuck
		if attempt <= maxRetries {
			debug.Log("⚠ Delivery not confirmed (attempt %d/%d): idle prompt visible, retrying send", attempt, maxRetries+1)
			if err := sendFunc(); err != nil {
				debug.Log("⚠ Retry send failed (attempt %d): %v", attempt, err)
				// Don't return error — continue to next attempt, the session state
				// might have changed (e.g., cooldown expired)
			}
		}
	}

	// All attempts exhausted
	debug.Log("✗ Delivery verification failed after %d attempts", maxRetries+1)
	return PromptDeliveryResult{Delivered: false, Attempt: 0, Method: ""}, nil
}

// extractKeywords pulls significant words from the prompt text for verification.
// Returns up to 3 keywords that are long enough to be meaningful and unlikely to
// match random pane content.
func extractKeywords(text string) []string {
	// Split on whitespace and punctuation-like characters
	words := strings.Fields(text)
	var keywords []string

	for _, w := range words {
		// Strip common punctuation
		w = strings.Trim(w, ".,;:!?\"'()[]{}#*-_=+/\\|<>@$%^&~`")
		// Only keep words that are meaningful (6+ chars, not common noise)
		if len(w) >= 6 && !isCommonWord(w) {
			keywords = append(keywords, strings.ToLower(w))
			if len(keywords) >= 3 {
				break
			}
		}
	}
	return keywords
}

// isCommonWord returns true for words that appear too frequently to be useful
// as delivery verification keywords.
func isCommonWord(word string) bool {
	common := map[string]bool{
		"please": true, "should": true, "would":  true, "could":  true,
		"their":  true, "there":  true, "these":  true, "those":  true,
		"which":  true, "where":  true, "about":  true, "after":  true,
		"before": true, "between": true, "through": true, "during": true,
	}
	return common[strings.ToLower(word)]
}

// keywordsFoundInContent checks if at least one keyword from the prompt appears
// in the captured pane content (case-insensitive).
func keywordsFoundInContent(keywords []string, content string) bool {
	lower := strings.ToLower(content)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
