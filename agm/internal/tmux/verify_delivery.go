package tmux

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// PromptDeliveryResult describes the outcome of a prompt delivery verification.
type PromptDeliveryResult struct {
	Delivered bool   // true if delivery was confirmed or conservatively accepted without a risky resend
	Attempt   int    // which attempt succeeded (1-indexed), 0 if all failed
	Method    string // how delivery was confirmed, or conservatively accepted when an idle Codex return is ambiguous
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
//  4. Ambiguous Codex idle return: a prompt without searchable keywords may
//     already have completed before the first capture, so it must not be resent
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
	return VerifyPromptDeliveryContext(context.Background(), sessionName, promptText, sendFunc, maxRetries)
}

// VerifyPromptDeliveryContext is the command-scoped delivery verifier. Caller
// cancellation stops verification backoff and prevents subsequent retry sends.
func VerifyPromptDeliveryContext(ctx context.Context, sessionName, promptText string, sendFunc func() error, maxRetries int) (PromptDeliveryResult, error) {
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
		if err := sleepWithContext(ctx, waitDuration); err != nil {
			return PromptDeliveryResult{}, err
		}

		// Capture pane content to check delivery status
		if err := ctx.Err(); err != nil {
			return PromptDeliveryResult{}, err
		}
		styledContent, err := CapturePaneLogicalANSIOutputContext(ctx, sessionName, 50)
		if err != nil {
			return PromptDeliveryResult{}, fmt.Errorf("capture-pane failed during delivery verification: %w", err)
		}
		if delivered, method := promptDeliveryEvidence(styledContent, keywords); delivered {
			debug.Log("✓ Delivery accepted (attempt %d, method: %s)", attempt, method)
			return PromptDeliveryResult{Delivered: true, Attempt: attempt, Method: method}, nil
		}

		// Delivery not confirmed — prompt might be stuck
		if attempt <= maxRetries {
			debug.Log("⚠ Delivery not confirmed (attempt %d/%d): idle prompt visible, retrying send", attempt, maxRetries+1)
			if err := ctx.Err(); err != nil {
				return PromptDeliveryResult{}, err
			}
			if err := sendFunc(); err != nil {
				if ctx.Err() != nil {
					return PromptDeliveryResult{}, ctx.Err()
				}
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

func promptDeliveryEvidence(styledContent string, keywords []string) (bool, string) {
	content := stripANSI(styledContent)
	if hasActiveSpinner(content) {
		return true, "processing"
	}
	if len(keywords) > 0 && keywordsFoundInContent(keywords, content) {
		return true, "keyword_match"
	}
	if !containsAnyHarnessPromptPattern(styledContent) {
		return true, "content_echo"
	}
	// The atomic sender already accepted the initial write. For a short prompt
	// with no searchable keyword, a fast Codex turn can finish before this first
	// capture and restore the styled idle composer. That state cannot distinguish
	// "never delivered" from "already completed", so retrying risks duplicating
	// completed work. Conservatively accept the ambiguous return without resend.
	if len(keywords) == 0 && IsCodexComposerReady(styledContent) {
		return true, "codex_idle_ambiguous"
	}
	return false, ""
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
		"please": true, "should": true, "would": true, "could": true,
		"their": true, "there": true, "these": true, "those": true,
		"which": true, "where": true, "about": true, "after": true,
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
