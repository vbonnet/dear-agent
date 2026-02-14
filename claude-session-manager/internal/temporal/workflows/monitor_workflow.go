package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// MonitorWorkflowInput contains parameters for starting a monitor workflow
type MonitorWorkflowInput struct {
	SessionID        string
	MonitorInterval  time.Duration // How often to check output
	EscalationRules  []EscalationRule
}

// EscalationRule defines a pattern that triggers escalation
type EscalationRule struct {
	Name        string   // Descriptive name (e.g., "Error detected")
	Patterns    []string // Regex patterns to match in output
	Severity    string   // "low", "medium", "high", "critical"
	NotifyAfter int      // Number of matches before escalating
}

// MonitorWorkflowState holds the current state of monitoring
type MonitorWorkflowState struct {
	SessionID        string
	IsMonitoring     bool
	LastCheckTime    time.Time
	TotalChecks      int
	EscalationsFound int
	MatchCounts      map[string]int // Pattern name -> count
}

// OutputLine represents a single line of output from the agent
type OutputLine struct {
	Timestamp time.Time
	Stream    string // "stdout" or "stderr"
	Content   string
}

// EscalationMatch represents a detected escalation
type EscalationMatch struct {
	SessionID   string
	RuleName    string
	Pattern     string
	Severity    string
	MatchedText string
	Timestamp   time.Time
	LineNumber  int
}

// MonitorSignals defines the signals for monitor workflow
const (
	SignalStartMonitoring = "startMonitoring"
	SignalStopMonitoring  = "stopMonitoring"
	SignalUpdateRules     = "updateRules"
)

// QueryMonitorState is the query name for getting monitor state
const QueryMonitorState = "getMonitorState"

// MonitorWorkflow monitors agent output for escalation patterns
// Runs continuously until stopped or session is archived
func MonitorWorkflow(ctx workflow.Context, input MonitorWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("MonitorWorkflow started", "sessionID", input.SessionID)

	// Default monitoring interval if not specified
	if input.MonitorInterval == 0 {
		input.MonitorInterval = 5 * time.Second
	}

	// Initialize workflow state
	state := MonitorWorkflowState{
		SessionID:        input.SessionID,
		IsMonitoring:     true,
		LastCheckTime:    workflow.Now(ctx),
		TotalChecks:      0,
		EscalationsFound: 0,
		MatchCounts:      make(map[string]int),
	}

	// Initialize match counts for each rule
	for _, rule := range input.EscalationRules {
		state.MatchCounts[rule.Name] = 0
	}

	// Register query handler for monitor state
	err := workflow.SetQueryHandler(ctx, QueryMonitorState, func() (MonitorWorkflowState, error) {
		return state, nil
	})
	if err != nil {
		logger.Error("Failed to register query handler", "error", err)
		return fmt.Errorf("failed to register query handler: %w", err)
	}

	// Signal channels
	startChannel := workflow.GetSignalChannel(ctx, SignalStartMonitoring)
	stopChannel := workflow.GetSignalChannel(ctx, SignalStopMonitoring)
	updateRulesChannel := workflow.GetSignalChannel(ctx, SignalUpdateRules)

	// Activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Main monitoring loop
	for {
		if state.IsMonitoring {
			// Fetch output from session
			var outputLines []OutputLine
			err := workflow.ExecuteActivity(ctx, "FetchSessionOutputActivity", input.SessionID).Get(ctx, &outputLines)
			if err != nil {
				logger.Error("FetchSessionOutputActivity failed", "error", err)
			} else {
				state.TotalChecks++
				state.LastCheckTime = workflow.Now(ctx)

				// Parse output for escalation patterns
				matches := parseOutputForEscalations(outputLines, input.EscalationRules, input.SessionID)

				for _, match := range matches {
					state.MatchCounts[match.RuleName]++

					// Check if we should trigger escalation based on rule
					for _, rule := range input.EscalationRules {
						if rule.Name == match.RuleName {
							if state.MatchCounts[match.RuleName] >= rule.NotifyAfter {
								state.EscalationsFound++
								logger.Info("Escalation triggered",
									"sessionID", input.SessionID,
									"ruleName", match.RuleName,
									"severity", match.Severity,
									"matchCount", state.MatchCounts[match.RuleName])

								// Start escalation workflow
								cwo := workflow.ChildWorkflowOptions{
									WorkflowID: fmt.Sprintf("escalation-%s-%s-%d",
										input.SessionID, match.RuleName, workflow.Now(ctx).Unix()),
								}
								childCtx := workflow.WithChildOptions(ctx, cwo)

								escalationInput := EscalationWorkflowInput{
									SessionID:   input.SessionID,
									RuleName:    match.RuleName,
									Severity:    match.Severity,
									MatchedText: match.MatchedText,
									Timestamp:   match.Timestamp,
								}

								var escalationResult string
								err := workflow.ExecuteChildWorkflow(childCtx, EscalationWorkflow, escalationInput).Get(ctx, &escalationResult)
								if err != nil {
									logger.Error("EscalationWorkflow failed", "error", err)
								} else {
									logger.Info("Escalation handled", "result", escalationResult)
								}

								// Reset counter after escalation
								state.MatchCounts[match.RuleName] = 0
							}
							break
						}
					}
				}
			}

			// Wait for next check or signal
			selector := workflow.NewSelector(ctx)

			// Add timer for next check
			timer := workflow.NewTimer(ctx, input.MonitorInterval)
			selector.AddFuture(timer, func(f workflow.Future) {
				// Timer expired, continue to next iteration
			})

			// Handle stop signal
			selector.AddReceive(stopChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				logger.Info("Stopping monitoring", "sessionID", input.SessionID)
				state.IsMonitoring = false
			})

			// Handle start signal (resume monitoring)
			selector.AddReceive(startChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				logger.Info("Starting monitoring", "sessionID", input.SessionID)
				state.IsMonitoring = true
			})

			// Handle update rules signal
			selector.AddReceive(updateRulesChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				var newRules []EscalationRule
				c.Receive(ctx, &newRules)
				logger.Info("Updating escalation rules", "sessionID", input.SessionID, "ruleCount", len(newRules))
				input.EscalationRules = newRules

				// Reset match counts for new rules
				state.MatchCounts = make(map[string]int)
				for _, rule := range newRules {
					state.MatchCounts[rule.Name] = 0
				}
			})

			selector.Select(ctx)

		} else {
			// Not monitoring - just wait for signals
			selector := workflow.NewSelector(ctx)

			selector.AddReceive(startChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				logger.Info("Resuming monitoring", "sessionID", input.SessionID)
				state.IsMonitoring = true
			})

			selector.AddReceive(stopChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				// Already stopped, end workflow
				logger.Info("Monitor workflow stopping", "sessionID", input.SessionID)
				return
			})

			selector.Select(ctx)
		}

		// Check if we should exit
		if !state.IsMonitoring {
			// Give one more chance to restart, otherwise exit
			selector := workflow.NewSelector(ctx)
			timer := workflow.NewTimer(ctx, 30*time.Second)

			selector.AddFuture(timer, func(f workflow.Future) {
				// Timeout - exit workflow
				logger.Info("Monitor workflow timeout, exiting", "sessionID", input.SessionID)
			})

			selector.AddReceive(startChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				logger.Info("Resuming monitoring before timeout", "sessionID", input.SessionID)
				state.IsMonitoring = true
			})

			selector.Select(ctx)

			if !state.IsMonitoring {
				break
			}
		}
	}

	logger.Info("MonitorWorkflow completed", "sessionID", input.SessionID,
		"totalChecks", state.TotalChecks, "escalationsFound", state.EscalationsFound)
	return nil
}

// parseOutputForEscalations checks output lines against escalation rules
func parseOutputForEscalations(lines []OutputLine, rules []EscalationRule, sessionID string) []EscalationMatch {
	matches := []EscalationMatch{}

	for i, line := range lines {
		for _, rule := range rules {
			for _, pattern := range rule.Patterns {
				// Simple substring matching (in production, use regex)
				if strings.Contains(strings.ToLower(line.Content), strings.ToLower(pattern)) {
					matches = append(matches, EscalationMatch{
						SessionID:   sessionID,
						RuleName:    rule.Name,
						Pattern:     pattern,
						Severity:    rule.Severity,
						MatchedText: line.Content,
						Timestamp:   line.Timestamp,
						LineNumber:  i + 1,
					})
				}
			}
		}
	}

	return matches
}
