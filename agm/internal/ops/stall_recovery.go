package ops

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"strings"
)

// RecoveryAction represents an action taken to recover from a stall.
type RecoveryAction struct {
	SessionName string // Target session
	ActionType  string // "nudge" | "alert_orchestrator" | "log_diagnostic" | "auto_approve"
	Description string // Human-readable action description
	Sent        bool   // Whether action succeeded
	Error       string // Error if failed
}

// StallRecovery handles recovery from detected stalls with retry tracking.
type StallRecovery struct {
	ctx                 *OpContext
	orchestratorName    string               // Name of orchestrator session to notify
	autoApprovePatterns []string             // Safe patterns to auto-approve
	retryTracker        *RetryTracker        // Tracks retry attempts with bounded retries
	bus                 eventbus.Broadcaster // Optional: publishes StallRecovered/StallEscalated events
}

// NewStallRecovery creates a new stall recovery handler with retry tracking.
func NewStallRecovery(ctx *OpContext, orchestratorName string) *StallRecovery {
	return &StallRecovery{
		ctx:              ctx,
		orchestratorName: orchestratorName,
		autoApprovePatterns: []string{
			"git",
			"ls",
			"cat",
			"grep",
			"find",
			"head",
			"tail",
			"pwd",
		},
		retryTracker: NewRetryTracker(getRetryBaseDir()),
	}
}

// Recover takes corrective action for a detected stall with retry tracking.
// Records failures and escalates to orchestrator after max retries exceeded.
func (sr *StallRecovery) Recover(ctx context.Context, event StallEvent) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
	}

	// Check if recovery should be attempted based on retry tracking
	canRetry, retryState, err := sr.checkCanRetry(event.SessionName)
	if err != nil {
		action.Error = fmt.Sprintf("retry check failed: %v", err)
		return action, err
	}

	if !canRetry && retryState != nil && retryState.AttemptCount > 0 {
		// Max retries exceeded - escalate to orchestrator
		return sr.escalateFailure(ctx, event, retryState)
	}

	// Include last error context if available
	errorContext := ""
	if retryState != nil && retryState.LastError != "" {
		errorContext = fmt.Sprintf(" [Previous attempt: %s]", retryState.LastError)
	}

	var (
		recovered RecoveryAction
		recovErr  error
	)
	switch event.StallType {
	case "permission_prompt":
		recovered, recovErr = sr.recoverPermissionPromptStall(ctx, event, errorContext)
	case "no_commit":
		recovered, recovErr = sr.recoverNoCommitStall(ctx, event, errorContext)
	case "error_loop":
		recovered, recovErr = sr.recoverErrorLoopStall(ctx, event, errorContext)
	default:
		return action, fmt.Errorf("unknown stall type: %s", event.StallType)
	}
	if recovErr != nil {
		sr.recordFailure(event.SessionName, recovErr.Error())
		recovered.Error = recovErr.Error()
	} else {
		sr.publishRecovered(event, recovered)
	}
	return recovered, recovErr
}

// recoverPermissionPromptStall handles recovery from permission prompt stalls.
func (sr *StallRecovery) recoverPermissionPromptStall(_ context.Context, event StallEvent, errorContext string) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "alert_orchestrator",
	}

	// Send alert to orchestrator
	if sr.orchestratorName == "" {
		return action, fmt.Errorf("no orchestrator session configured")
	}

	msg := fmt.Sprintf("⚠️ PERMISSION_PROMPT stall detected: %s has been stuck for %v%s",
		event.SessionName, formatDuration(event.Duration), errorContext)

	result, err := SendMessage(sr.ctx, &SendMessageRequest{
		Recipient: sr.orchestratorName,
		Message:   msg,
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	action.Sent = result.Delivered
	action.Description = fmt.Sprintf("Sent alert to %s", sr.orchestratorName)
	return action, nil
}

// recoverNoCommitStall handles recovery from no-commit stalls.
func (sr *StallRecovery) recoverNoCommitStall(_ context.Context, event StallEvent, errorContext string) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "nudge",
	}

	// Send nudge message to worker
	msg := fmt.Sprintf("🔔 Nudge: No commits detected in %v. Check for blocking errors.%s", formatDuration(event.Duration), errorContext)

	result, err := SendMessage(sr.ctx, &SendMessageRequest{
		Recipient: event.SessionName,
		Message:   msg,
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	action.Sent = result.Delivered
	action.Description = "Sent nudge message to worker"
	return action, nil
}

// recoverErrorLoopStall handles recovery from error loop stalls.
func (sr *StallRecovery) recoverErrorLoopStall(_ context.Context, event StallEvent, errorContext string) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "log_diagnostic",
	}

	if sr.orchestratorName == "" {
		// Just log locally if no orchestrator
		action.Description = fmt.Sprintf("Error loop detected: %s%s", event.Evidence, errorContext)
		action.Sent = true
		return action, nil
	}

	// Send diagnostic to orchestrator
	msg := fmt.Sprintf("🔄 ERROR_LOOP detected in %s:\n%s%s", event.SessionName, event.Evidence, errorContext)

	result, err := SendMessage(sr.ctx, &SendMessageRequest{
		Recipient: sr.orchestratorName,
		Message:   msg,
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	action.Sent = result.Delivered
	action.Description = fmt.Sprintf("Sent diagnostic to %s", sr.orchestratorName)
	return action, nil
}

// isSafeForAutoApproval checks if an error message is safe to auto-approve.
func (sr *StallRecovery) isSafeForAutoApproval(evidence string) bool {
	lowerEvidence := strings.ToLower(evidence)
	for _, pattern := range sr.autoApprovePatterns {
		if strings.Contains(lowerEvidence, pattern) {
			return true
		}
	}
	return false
}

// checkCanRetry checks if a session can attempt recovery based on retry tracking.
// Returns (canRetry, retryState, error).
func (sr *StallRecovery) checkCanRetry(sessionName string) (bool, *RetryState, error) {
	state, err := sr.retryTracker.LoadRetryState(sessionName)
	if err != nil {
		return false, nil, err
	}

	// If no attempts recorded, allow first attempt
	if state.AttemptCount == 0 {
		return true, state, nil
	}

	// Check if ready for retry based on backoff
	canRetry, err := sr.retryTracker.CanRetryNow(sessionName)
	return canRetry, state, err
}

// recordFailure records a failed recovery attempt.
func (sr *StallRecovery) recordFailure(sessionName string, errorMsg string) error {
	_, _, err := sr.retryTracker.RecordRetryAttempt(sessionName, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to record retry attempt: %w", err)
	}
	return nil
}

// escalateFailure escalates a session to the orchestrator after max retries exceeded.
func (sr *StallRecovery) escalateFailure(_ context.Context, event StallEvent, retryState *RetryState) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "escalate",
	}

	if sr.orchestratorName == "" {
		return action, fmt.Errorf("no orchestrator session configured for escalation")
	}

	// Create escalation message with full context
	msg := fmt.Sprintf("🚨 ESCALATION: %s failed after %d retry attempts\n\nStall Type: %s\nDuration: %v\nLast Error: %s\nInitial Attempt: %v",
		event.SessionName,
		retryState.AttemptCount,
		event.StallType,
		formatDuration(event.Duration),
		retryState.LastError,
		retryState.LastAttempt,
	)

	result, err := SendMessage(sr.ctx, &SendMessageRequest{
		Recipient: sr.orchestratorName,
		Message:   msg,
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	action.Sent = result.Delivered
	action.Description = fmt.Sprintf("Escalated to %s after %d failed attempts", sr.orchestratorName, retryState.AttemptCount)

	sr.publishEscalated(event, retryState.AttemptCount)
	// Record as final failure - don't record more attempts after escalation
	return action, nil
}

// SetBus sets the event bus broadcaster for publishing recovery/escalation events.
func (sr *StallRecovery) SetBus(bus eventbus.Broadcaster) {
	sr.bus = bus
}

// publishRecovered publishes a StallRecovered event to the EventBus.
func (sr *StallRecovery) publishRecovered(event StallEvent, action RecoveryAction) {
	if sr.bus == nil {
		return
	}
	busEvent, err := eventbus.NewEvent(eventbus.EventStallRecovered, event.SessionName, eventbus.StallRecoveredPayload{
		StallType:      event.StallType,
		Session:        event.SessionName,
		RecoveryAction: action.ActionType,
		Duration:       event.Duration,
	})
	if err != nil {
		return
	}
	sr.bus.Broadcast(busEvent)
}

// publishEscalated publishes a StallEscalated event to the EventBus.
func (sr *StallRecovery) publishEscalated(event StallEvent, attemptCount int) {
	if sr.bus == nil {
		return
	}
	busEvent, err := eventbus.NewEvent(eventbus.EventStallEscalated, event.SessionName, eventbus.StallEscalatedPayload{
		StallType:    event.StallType,
		Session:      event.SessionName,
		Reason:       fmt.Sprintf("max retries exceeded after %d attempts", attemptCount),
		AttemptCount: attemptCount,
	})
	if err != nil {
		return
	}
	sr.bus.Broadcast(busEvent)
}
