package ops

import (
	"fmt"
	"os"
	"time"
)

// getRetryBaseDir returns the base directory for retry state storage.
// Uses AGM_RETRY_BASE_DIR env var if set (for testing), otherwise home directory.
func getRetryBaseDir() string {
	if dir := os.Getenv("AGM_RETRY_BASE_DIR"); dir != "" {
		return dir
	}
	homeDir, _ := os.UserHomeDir()
	return homeDir
}

// RetrySessionRequest defines the input for manually retrying a session.
type RetrySessionRequest struct {
	SessionName string `json:"session_name"`
}

// RetrySessionResult is the output of RetrySession.
type RetrySessionResult struct {
	Operation   string     `json:"operation"`
	SessionName string     `json:"session_name"`
	RetryState  *RetryState `json:"retry_state"`
	Status      string     `json:"status"`
	Description string     `json:"description,omitempty"`
}

// RetrySession manually retries a session with error context from previous failure.
// It checks retry state, enforces backoff, and includes error context in recovery.
func RetrySession(ctx *OpContext, req *RetrySessionRequest) (*RetrySessionResult, error) {
	if req.SessionName == "" {
		return nil, fmt.Errorf("session_name is required")
	}

	result := &RetrySessionResult{
		Operation:   "session_retry",
		SessionName: req.SessionName,
	}

	// Load current retry state - use home directory as base or config override for testing
	baseDir := getRetryBaseDir()
	retryTracker := NewRetryTracker(baseDir)
	retryState, err := retryTracker.LoadRetryState(req.SessionName)
	if err != nil {
		return result, fmt.Errorf("failed to load retry state: %w", err)
	}

	result.RetryState = retryState

	// Check if backoff has elapsed
	canRetryNow, err := retryTracker.CanRetryNow(req.SessionName)
	if err != nil {
		return result, fmt.Errorf("failed to check retry status: %w", err)
	}

	// Provide status message
	switch {
	case retryState.AttemptCount == 0:
		result.Status = "ready"
		result.Description = "No previous attempts recorded. This session is ready for first attempt."
		return result, nil

	case retryState.AttemptCount >= 3: // MaxRetries
		result.Status = "max_retries_exceeded"
		result.Description = fmt.Sprintf("Max retries (%d) exceeded. Session should be escalated to orchestrator.", 3)
		return result, nil

	case !canRetryNow:
		result.Status = "backoff_not_elapsed"
		timeUntilRetry := time.Until(retryState.NextRetryAt)
		result.Description = fmt.Sprintf("Backoff period not elapsed. Retry available in %v (at %s)",
			timeUntilRetry, retryState.NextRetryAt.Format(time.RFC3339))
		return result, nil

	case canRetryNow:
		// Backoff has elapsed, session is ready for retry
		result.Status = "ready_for_retry"
		result.Description = fmt.Sprintf("Backoff period elapsed. Session is ready for retry attempt %d of %d. Previous error: %s",
			retryState.AttemptCount+1, 3, retryState.LastError)
		return result, nil

	default:
		result.Status = "unknown"
		result.Description = "Unable to determine retry status"
		return result, nil
	}
}

// RetrySessionWithMessage extends RetrySession to also send a retry nudge message to the session.
// This is useful for notifying a stalled session that it's being retried.
func RetrySessionWithMessage(ctx *OpContext, req *RetrySessionRequest) (*RetrySessionResult, error) {
	// First, get the retry status
	result, err := RetrySession(ctx, req)
	if err != nil {
		return result, err
	}

	// If session is ready for retry, send a nudge message
	if result.Status == "ready_for_retry" {
		msg := fmt.Sprintf("🔄 Retry attempt %d: Retrying after previous error: %s",
			result.RetryState.AttemptCount+1, result.RetryState.LastError)

		_, msgErr := SendMessage(ctx, &SendMessageRequest{
			Recipient: req.SessionName,
			Message:   msg,
		})

		if msgErr != nil {
			result.Description = fmt.Sprintf("%s (Note: Failed to send retry message: %v)",
				result.Description, msgErr)
		} else {
			result.Description = fmt.Sprintf("%s (Retry message sent)", result.Description)
		}
	}

	return result, nil
}
