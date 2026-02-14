package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// EscalationWorkflowInput contains parameters for starting an escalation workflow
type EscalationWorkflowInput struct {
	SessionID      string
	RuleName       string
	Severity       string
	MatchedText    string
	Timestamp      time.Time
	NotificationChannels []NotificationChannel
}

// NotificationChannel defines where to send escalation notifications
type NotificationChannel struct {
	Type     string // "email", "slack", "webhook", "log"
	Target   string // Email address, Slack channel, webhook URL, etc.
	Priority int    // Lower number = higher priority
}

// EscalationWorkflowState holds the current state of escalation handling
type EscalationWorkflowState struct {
	SessionID          string
	RuleName           string
	Severity           string
	Status             string // "pending", "notifying", "completed", "failed"
	NotificationsSent  int
	NotificationsFailed int
	StartedAt          time.Time
	CompletedAt        time.Time
	RetryCount         int
}

// NotificationResult represents the result of sending a notification
type NotificationResult struct {
	Channel   NotificationChannel
	Success   bool
	Message   string
	Timestamp time.Time
}

// QueryEscalationState is the query name for getting escalation state
const QueryEscalationState = "getEscalationState"

// EscalationWorkflow handles escalation notifications
// Sends notifications through configured channels based on severity
func EscalationWorkflow(ctx workflow.Context, input EscalationWorkflowInput) (string, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("EscalationWorkflow started",
		"sessionID", input.SessionID,
		"ruleName", input.RuleName,
		"severity", input.Severity)

	// Initialize workflow state
	state := EscalationWorkflowState{
		SessionID:          input.SessionID,
		RuleName:           input.RuleName,
		Severity:           input.Severity,
		Status:             "pending",
		NotificationsSent:  0,
		NotificationsFailed: 0,
		StartedAt:          workflow.Now(ctx),
		RetryCount:         0,
	}

	// Register query handler for escalation state
	err := workflow.SetQueryHandler(ctx, QueryEscalationState, func() (EscalationWorkflowState, error) {
		return state, nil
	})
	if err != nil {
		logger.Error("Failed to register query handler", "error", err)
		return "", fmt.Errorf("failed to register query handler: %w", err)
	}

	// Set default notification channels if none provided
	if len(input.NotificationChannels) == 0 {
		input.NotificationChannels = getDefaultNotificationChannels(input.Severity)
	}

	// Activity options with retries based on severity
	retryPolicy := getRetryPolicyForSeverity(input.Severity)
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 60 * time.Second,
		RetryPolicy:         retryPolicy,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Log escalation event
	state.Status = "notifying"
	err = workflow.ExecuteActivity(ctx, "LogEscalationActivity", input).Get(ctx, nil)
	if err != nil {
		logger.Warn("LogEscalationActivity failed", "error", err)
		// Continue even if logging fails
	}

	// Send notifications to all configured channels
	results := []NotificationResult{}

	for _, channel := range input.NotificationChannels {
		logger.Info("Sending notification",
			"sessionID", input.SessionID,
			"channel", channel.Type,
			"target", channel.Target)

		notificationInput := NotificationInput{
			SessionID:   input.SessionID,
			RuleName:    input.RuleName,
			Severity:    input.Severity,
			MatchedText: input.MatchedText,
			Timestamp:   input.Timestamp,
			Channel:     channel,
		}

		var result NotificationResult
		err := workflow.ExecuteActivity(ctx, "SendNotificationActivity", notificationInput).Get(ctx, &result)

		if err != nil {
			logger.Error("SendNotificationActivity failed",
				"channel", channel.Type,
				"target", channel.Target,
				"error", err)
			state.NotificationsFailed++
			state.RetryCount++
			results = append(results, NotificationResult{
				Channel:   channel,
				Success:   false,
				Message:   fmt.Sprintf("Failed: %v", err),
				Timestamp: workflow.Now(ctx),
			})
		} else {
			state.NotificationsSent++
			results = append(results, result)
			logger.Info("Notification sent successfully",
				"channel", channel.Type,
				"target", channel.Target)
		}
	}

	// Handle critical severity - ensure at least one notification succeeded
	if input.Severity == "critical" && state.NotificationsSent == 0 {
		logger.Error("Critical escalation: all notifications failed",
			"sessionID", input.SessionID,
			"ruleName", input.RuleName)

		// Try fallback notification
		fallbackChannel := NotificationChannel{
			Type:     "log",
			Target:   "escalation-fallback",
			Priority: 0,
		}

		fallbackInput := NotificationInput{
			SessionID:   input.SessionID,
			RuleName:    input.RuleName,
			Severity:    "critical-fallback",
			MatchedText: fmt.Sprintf("CRITICAL ESCALATION FAILED: %s - %s", input.RuleName, input.MatchedText),
			Timestamp:   input.Timestamp,
			Channel:     fallbackChannel,
		}

		err := workflow.ExecuteActivity(ctx, "SendNotificationActivity", fallbackInput).Get(ctx, nil)
		if err != nil {
			logger.Error("Fallback notification also failed", "error", err)
			state.Status = "failed"
			state.CompletedAt = workflow.Now(ctx)
			return "escalation failed - all notifications failed", fmt.Errorf("critical escalation failed: all notifications failed")
		}
		state.NotificationsSent++
	}

	// Update escalation status
	if state.NotificationsSent > 0 {
		state.Status = "completed"
	} else {
		state.Status = "failed"
	}
	state.CompletedAt = workflow.Now(ctx)

	// Store escalation result
	escalationRecord := EscalationRecord{
		SessionID:          input.SessionID,
		RuleName:           input.RuleName,
		Severity:           input.Severity,
		MatchedText:        input.MatchedText,
		Timestamp:          input.Timestamp,
		NotificationsSent:  state.NotificationsSent,
		NotificationsFailed: state.NotificationsFailed,
		Results:            results,
		CompletedAt:        state.CompletedAt,
	}

	err = workflow.ExecuteActivity(ctx, "StoreEscalationRecordActivity", escalationRecord).Get(ctx, nil)
	if err != nil {
		logger.Warn("StoreEscalationRecordActivity failed", "error", err)
		// Continue even if storage fails
	}

	resultMessage := fmt.Sprintf("Escalation handled: %d notifications sent, %d failed",
		state.NotificationsSent, state.NotificationsFailed)

	logger.Info("EscalationWorkflow completed",
		"sessionID", input.SessionID,
		"status", state.Status,
		"notificationsSent", state.NotificationsSent,
		"notificationsFailed", state.NotificationsFailed)

	if state.Status == "failed" {
		return resultMessage, fmt.Errorf("escalation failed: no notifications sent")
	}

	return resultMessage, nil
}

// NotificationInput contains parameters for sending a notification
type NotificationInput struct {
	SessionID   string
	RuleName    string
	Severity    string
	MatchedText string
	Timestamp   time.Time
	Channel     NotificationChannel
}

// EscalationRecord stores the complete record of an escalation event
type EscalationRecord struct {
	SessionID          string
	RuleName           string
	Severity           string
	MatchedText        string
	Timestamp          time.Time
	NotificationsSent  int
	NotificationsFailed int
	Results            []NotificationResult
	CompletedAt        time.Time
}

// getDefaultNotificationChannels returns default notification channels based on severity
func getDefaultNotificationChannels(severity string) []NotificationChannel {
	switch severity {
	case "critical":
		return []NotificationChannel{
			{Type: "log", Target: "escalation-critical", Priority: 0},
			{Type: "webhook", Target: "default-webhook", Priority: 1},
		}
	case "high":
		return []NotificationChannel{
			{Type: "log", Target: "escalation-high", Priority: 0},
		}
	case "medium":
		return []NotificationChannel{
			{Type: "log", Target: "escalation-medium", Priority: 0},
		}
	case "low":
		return []NotificationChannel{
			{Type: "log", Target: "escalation-low", Priority: 0},
		}
	default:
		return []NotificationChannel{
			{Type: "log", Target: "escalation-default", Priority: 0},
		}
	}
}

// getRetryPolicyForSeverity returns appropriate retry policy based on severity
func getRetryPolicyForSeverity(severity string) *temporal.RetryPolicy {
	switch severity {
	case "critical":
		return &temporal.RetryPolicy{
			MaximumAttempts:        5,
			InitialInterval:        1 * time.Second,
			MaximumInterval:        30 * time.Second,
			BackoffCoefficient:     2.0,
			NonRetryableErrorTypes: []string{},
		}
	case "high":
		return &temporal.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    2 * time.Second,
			MaximumInterval:    20 * time.Second,
			BackoffCoefficient: 2.0,
		}
	case "medium", "low":
		return &temporal.RetryPolicy{
			MaximumAttempts:    2,
			InitialInterval:    5 * time.Second,
			MaximumInterval:    15 * time.Second,
			BackoffCoefficient: 1.5,
		}
	default:
		return &temporal.RetryPolicy{
			MaximumAttempts: 1,
		}
	}
}
