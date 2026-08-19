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
	router              *AlertRouter         // Optional: nil builds the default router lazily
	// orchestratorTarget resolves the live relay target. Nil falls back to
	// the name this recovery was constructed with.
	orchestratorTarget func(string) string
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
func (sr *StallRecovery) recoverPermissionPromptStall(ctx context.Context, event StallEvent, errorContext string) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "alert_orchestrator",
	}

	msg := fmt.Sprintf("⚠️ PERMISSION_PROMPT stall detected: %s has been stuck for %v%s",
		event.SessionName, formatDuration(event.Duration), errorContext)
	rec, err := sr.alertRouter().Route(ctx, AlertRequest{
		Kind:          "stall.permission_prompt",
		Source:        "agm-watch-stalled",
		Title:         "Permission prompt stall",
		Body:          msg,
		Subject:       event.SessionName,
		Severity:      AlertSeverityCritical,
		Actionability: AlertAgentActionable,
		Target:        sr.resolveOrchestrator(),
		OccurredAt:    event.DetectedAt,
		Meta: map[string]any{
			"stall_type": event.StallType,
			"duration":   event.Duration.String(),
			"evidence":   event.Evidence,
		},
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	// A queued alert reached nobody, and a suppressed duplicate of a queued
	// alert reached nobody either. Reporting either as sent would let
	// Recover publish StallRecovered over an unresolved critical stall.
	action.Sent = rec.Delivered()
	action.Description = fmt.Sprintf("Alert status %s target %s", rec.Status, rec.Target)
	if !action.Sent {
		return action, fmt.Errorf("permission-prompt alert not delivered (status %s): %s", rec.Status, alertFailureReason(rec))
	}
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
func (sr *StallRecovery) recoverErrorLoopStall(ctx context.Context, event StallEvent, errorContext string) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "log_diagnostic",
	}

	msg := fmt.Sprintf("🔄 ERROR_LOOP detected in %s:\n%s%s", event.SessionName, event.Evidence, errorContext)
	rec, err := sr.alertRouter().Route(ctx, AlertRequest{
		Kind:          "stall.error_loop",
		Source:        "agm-watch-stalled",
		Title:         "Error loop diagnostic",
		Body:          msg,
		Subject:       event.SessionName,
		Severity:      AlertSeverityWarning,
		Actionability: AlertAgentActionable,
		Target:        sr.resolveOrchestrator(),
		OccurredAt:    event.DetectedAt,
		Meta: map[string]any{
			"stall_type": event.StallType,
			"evidence":   event.Evidence,
		},
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	// Queued is the router's word for "delivery failed"; counting it as
	// sent would mark the recovery successful and stop the retry tracker
	// from ever advancing toward max-retry escalation.
	action.Sent = rec.Delivered()
	action.Description = fmt.Sprintf("Diagnostic status %s target %s", rec.Status, rec.Target)
	if !action.Sent {
		return action, fmt.Errorf("error-loop diagnostic not delivered (status %s): %s", rec.Status, alertFailureReason(rec))
	}
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
func (sr *StallRecovery) escalateFailure(ctx context.Context, event StallEvent, retryState *RetryState) (RecoveryAction, error) {
	action := RecoveryAction{
		SessionName: event.SessionName,
		ActionType:  "escalate",
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

	rec, err := sr.alertRouter().Route(ctx, AlertRequest{
		Kind:          "stall.escalation",
		Source:        "agm-watch-stalled",
		Title:         "Stall recovery escalation",
		Body:          msg,
		Subject:       event.SessionName,
		Severity:      AlertSeverityCritical,
		Actionability: AlertAgentActionable,
		Target:        sr.resolveOrchestrator(),
		OccurredAt:    event.DetectedAt,
	})

	if err != nil {
		action.Error = err.Error()
		return action, err
	}

	action.Sent = rec.Delivered()
	action.Description = fmt.Sprintf("Escalated status %s target %s after %d failed attempts", rec.Status, rec.Target, retryState.AttemptCount)
	if !action.Sent {
		return action, fmt.Errorf("escalation not delivered (status %s): %s", rec.Status, alertFailureReason(rec))
	}

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

// alertFailureReason renders why an alert did not reach a recipient.
func alertFailureReason(rec AlertRecord) string {
	if rec.Error != "" {
		return rec.Error
	}
	return "no reason recorded"
}

// SetAlertRouter overrides the router stall recovery delivers through.
//
// Tests must set it. The default router writes to the host-wide
// ~/.agm/alerts/queue.jsonl, so without injection `go test ./agm/internal/ops`
// appends synthetic alerts to the developer's real queue, and tests
// suppress one another through that shared persisted state.
func (sr *StallRecovery) SetAlertRouter(router *AlertRouter) {
	sr.router = router
}

// alertRouter returns the injected router, or builds the default one.
func (sr *StallRecovery) alertRouter() *AlertRouter {
	if sr.router != nil {
		return sr.router
	}
	sr.router = NewAlertRouter(sr.ctx)
	return sr.router
}

// SetOrchestratorTargetResolver overrides how a stalled session's alert
// target is resolved, so the target can be looked up live instead of being
// fixed at construction.
func (sr *StallRecovery) SetOrchestratorTargetResolver(resolve func(string) string) {
	sr.orchestratorTarget = resolve
}

// resolveOrchestrator reports the preferred recipient for this recovery's
// alerts.
//
// This is the seam where the two halves of routing meet: it yields the
// live relay target (state file, then AGM_COMPLETION_RELAY_TARGET, then
// the --orchestrator fallback), and the alert router then treats that as
// its explicit first candidate, checking it for liveness before falling
// through to discovery and finally to the durable queue. An operator
// retargeting relay therefore steers stall alerts too, without the router
// losing its ability to find someone else when that target is dead.
func (sr *StallRecovery) resolveOrchestrator() string {
	if sr.orchestratorTarget != nil {
		return sr.orchestratorTarget(sr.orchestratorName)
	}
	return sr.orchestratorName
}
