package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// SessionState represents the lifecycle state of a session
type SessionState string

const (
	SessionStateCreated  SessionState = "created"
	SessionStateActive   SessionState = "active"
	SessionStateStopped  SessionState = "stopped"
	SessionStateArchived SessionState = "archived"
)

// SessionWorkflowInput contains parameters for starting a session workflow
type SessionWorkflowInput struct {
	SessionID   string
	SessionName string
	WorkingDir  string
	Agent       string // claude, gemini, gpt, etc.
	Project     string
	Tags        []string
}

// SessionWorkflowState holds the current state of the session workflow
type SessionWorkflowState struct {
	SessionID   string
	SessionName string
	State       SessionState
	WorkingDir  string
	Agent       string
	Project     string
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	AttachedClients int
}

// SessionSignals defines the signals that can be sent to the session workflow
const (
	SignalActivate = "activate"
	SignalStop     = "stop"
	SignalArchive  = "archive"
	SignalAttach   = "attach"
	SignalDetach   = "detach"
)

// QuerySessionState is the query name for getting session state
const QuerySessionState = "getSessionState"

// SessionWorkflow manages the lifecycle of an AGM session
// State transitions: created -> active -> stopped -> archived
func SessionWorkflow(ctx workflow.Context, input SessionWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("SessionWorkflow started", "sessionID", input.SessionID, "sessionName", input.SessionName)

	// Initialize workflow state
	state := SessionWorkflowState{
		SessionID:   input.SessionID,
		SessionName: input.SessionName,
		State:       SessionStateCreated,
		WorkingDir:  input.WorkingDir,
		Agent:       input.Agent,
		Project:     input.Project,
		Tags:        input.Tags,
		CreatedAt:   workflow.Now(ctx),
		UpdatedAt:   workflow.Now(ctx),
		AttachedClients: 0,
	}

	// Register query handler for session state
	err := workflow.SetQueryHandler(ctx, QuerySessionState, func() (SessionWorkflowState, error) {
		return state, nil
	})
	if err != nil {
		logger.Error("Failed to register query handler", "error", err)
		return fmt.Errorf("failed to register query handler: %w", err)
	}

	// Signal channels
	activateChannel := workflow.GetSignalChannel(ctx, SignalActivate)
	stopChannel := workflow.GetSignalChannel(ctx, SignalStop)
	archiveChannel := workflow.GetSignalChannel(ctx, SignalArchive)
	attachChannel := workflow.GetSignalChannel(ctx, SignalAttach)
	detachChannel := workflow.GetSignalChannel(ctx, SignalDetach)

	// Execute CreateSession activity
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var createResult string
	err = workflow.ExecuteActivity(ctx, "CreateSessionActivity", input).Get(ctx, &createResult)
	if err != nil {
		logger.Error("CreateSessionActivity failed", "error", err)
		return fmt.Errorf("failed to create session: %w", err)
	}
	logger.Info("Session created", "result", createResult)

	// Automatically transition to active state after creation
	state.State = SessionStateActive
	state.UpdatedAt = workflow.Now(ctx)
	logger.Info("Session transitioned to active state", "sessionID", input.SessionID)

	// Main event loop - wait for signals
	for {
		selector := workflow.NewSelector(ctx)

		// Handle activate signal
		selector.AddReceive(activateChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			if state.State != SessionStateActive {
				logger.Info("Activating session", "sessionID", state.SessionID, "previousState", state.State)
				state.State = SessionStateActive
				state.UpdatedAt = workflow.Now(ctx)

				// Execute activity to activate session
				err := workflow.ExecuteActivity(ctx, "ActivateSessionActivity", state.SessionID).Get(ctx, nil)
				if err != nil {
					logger.Error("ActivateSessionActivity failed", "error", err)
				}
			}
		})

		// Handle stop signal
		selector.AddReceive(stopChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			if state.State == SessionStateActive {
				logger.Info("Stopping session", "sessionID", state.SessionID)
				state.State = SessionStateStopped
				state.UpdatedAt = workflow.Now(ctx)

				// Execute activity to stop session
				err := workflow.ExecuteActivity(ctx, "StopSessionActivity", state.SessionID).Get(ctx, nil)
				if err != nil {
					logger.Error("StopSessionActivity failed", "error", err)
				}
			}
		})

		// Handle archive signal
		selector.AddReceive(archiveChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			logger.Info("Archiving session", "sessionID", state.SessionID, "previousState", state.State)
			state.State = SessionStateArchived
			state.UpdatedAt = workflow.Now(ctx)

			// Execute activity to archive session
			err := workflow.ExecuteActivity(ctx, "ArchiveSessionActivity", state.SessionID).Get(ctx, nil)
			if err != nil {
				logger.Error("ArchiveSessionActivity failed", "error", err)
				// Continue even if archive fails - we still want to mark as archived
			}

			// End workflow after archiving
			logger.Info("Session workflow completed", "sessionID", state.SessionID)
			return
		})

		// Handle attach signal
		selector.AddReceive(attachChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			state.AttachedClients++
			state.UpdatedAt = workflow.Now(ctx)
			logger.Info("Client attached", "sessionID", state.SessionID, "attachedClients", state.AttachedClients)
		})

		// Handle detach signal
		selector.AddReceive(detachChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			if state.AttachedClients > 0 {
				state.AttachedClients--
			}
			state.UpdatedAt = workflow.Now(ctx)
			logger.Info("Client detached", "sessionID", state.SessionID, "attachedClients", state.AttachedClients)
		})

		selector.Select(ctx)

		// If archived, exit the workflow
		if state.State == SessionStateArchived {
			break
		}
	}

	logger.Info("SessionWorkflow completed", "sessionID", input.SessionID, "finalState", state.State)
	return nil
}
