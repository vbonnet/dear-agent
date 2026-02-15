package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// MCPServiceState represents the lifecycle state of an MCP service
type MCPServiceState string

const (
	MCPStateStopped  MCPServiceState = "stopped"
	MCPStateStarting MCPServiceState = "starting"
	MCPStateRunning  MCPServiceState = "running"
	MCPStateStopping MCPServiceState = "stopping"
)

// MCPServiceConfig contains configuration for starting an MCP service
type MCPServiceConfig struct {
	Name           string            // e.g., "googledocs"
	MCPCommand     string            // The MCP server command to proxy
	Port           int               // HTTP server port (e.g., 8001)
	ServerPath     string            // Path to the HTTP server script
	Environment    map[string]string // Additional environment variables
	HealthCheckURL string            // Health check endpoint URL
}

// MCPServiceWorkflowState holds the current state of the MCP service workflow
type MCPServiceWorkflowState struct {
	Name           string
	State          MCPServiceState
	MCPCommand     string
	Port           int
	PID            int
	StartedAt      time.Time
	LastHealthAt   time.Time
	FailureCount   int
	HealthCheckURL string
}

// MCPSignals defines the signals that can be sent to the MCP service workflow
const (
	SignalStartMCP   = "startMCP"
	SignalStopMCP    = "stopMCP"
	SignalRestartMCP = "restartMCP"
	SignalHealthMCP  = "healthCheckMCP"
)

// QueryMCPState is the query name for getting MCP state
const QueryMCPState = "getMCPState"

// MCPServiceWorkflow manages the lifecycle of a global MCP HTTP server process
// State transitions: stopped -> starting -> running -> stopping -> stopped
// This workflow continuously monitors the MCP server health and handles restarts
func MCPServiceWorkflow(ctx workflow.Context, config MCPServiceConfig) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("MCPServiceWorkflow started", "name", config.Name, "port", config.Port)

	// Validate configuration
	if config.Name == "" {
		return fmt.Errorf("MCP service name cannot be empty")
	}
	if config.MCPCommand == "" {
		return fmt.Errorf("MCP command cannot be empty")
	}
	if config.Port == 0 {
		return fmt.Errorf("port cannot be zero")
	}

	// Set default health check URL if not provided
	if config.HealthCheckURL == "" {
		config.HealthCheckURL = fmt.Sprintf("http://localhost:%d/health", config.Port)
	}

	// Initialize workflow state
	state := MCPServiceWorkflowState{
		Name:           config.Name,
		State:          MCPStateStopped,
		MCPCommand:     config.MCPCommand,
		Port:           config.Port,
		PID:            0,
		FailureCount:   0,
		HealthCheckURL: config.HealthCheckURL,
	}

	// Register query handler for MCP state
	err := workflow.SetQueryHandler(ctx, QueryMCPState, func() (MCPServiceWorkflowState, error) {
		return state, nil
	})
	if err != nil {
		logger.Error("Failed to register query handler", "error", err)
		return fmt.Errorf("failed to register query handler: %w", err)
	}

	// Signal channels
	startChannel := workflow.GetSignalChannel(ctx, SignalStartMCP)
	stopChannel := workflow.GetSignalChannel(ctx, SignalStopMCP)
	restartChannel := workflow.GetSignalChannel(ctx, SignalRestartMCP)
	healthChannel := workflow.GetSignalChannel(ctx, SignalHealthMCP)

	// Activity options with retry policy
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	activityCtx := workflow.WithActivityOptions(ctx, ao)

	// Main event loop
	for {
		selector := workflow.NewSelector(ctx)

		// Handle start signal
		selector.AddReceive(startChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			if state.State == MCPStateStopped {
				logger.Info("Starting MCP service", "name", state.Name)
				state.State = MCPStateStarting

				// Execute StartMCP activity
				var startResult StartMCPResult
				err := workflow.ExecuteActivity(activityCtx, "StartMCPActivity", StartMCPInput{
					Name:        config.Name,
					MCPCommand:  config.MCPCommand,
					Port:        config.Port,
					ServerPath:  config.ServerPath,
					Environment: config.Environment,
				}).Get(ctx, &startResult)

				if err != nil {
					logger.Error("StartMCPActivity failed", "error", err)
					state.State = MCPStateStopped
					state.FailureCount++
				} else {
					logger.Info("MCP service started", "pid", startResult.PID, "port", startResult.Port)
					state.State = MCPStateRunning
					state.PID = startResult.PID
					state.StartedAt = startResult.StartedAt
					state.FailureCount = 0
				}
			} else {
				logger.Info("MCP service already running", "state", state.State)
			}
		})

		// Handle stop signal
		selector.AddReceive(stopChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			if state.State == MCPStateRunning {
				logger.Info("Stopping MCP service", "name", state.Name, "pid", state.PID)
				state.State = MCPStateStopping

				// Execute StopMCP activity
				var stopResult StopMCPResult
				err := workflow.ExecuteActivity(activityCtx, "StopMCPActivity", StopMCPInput{
					Name:        state.Name,
					PID:         state.PID,
					GracePeriod: 10 * time.Second,
					ForceKill:   true,
				}).Get(ctx, &stopResult)

				if err != nil {
					logger.Error("StopMCPActivity failed", "error", err)
					// Still mark as stopped
				} else {
					logger.Info("MCP service stopped", "graceful", stopResult.GracefulExit)
				}

				state.State = MCPStateStopped
				state.PID = 0
			} else if state.State == MCPStateStopped {
				// Already stopped, exit workflow
				logger.Info("MCP service already stopped, exiting workflow")
				return
			}
		})

		// Handle restart signal
		selector.AddReceive(restartChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			logger.Info("Restarting MCP service", "name", state.Name)

			// Stop if running
			if state.State == MCPStateRunning && state.PID > 0 {
				state.State = MCPStateStopping
				var stopResult StopMCPResult
				err := workflow.ExecuteActivity(activityCtx, "StopMCPActivity", StopMCPInput{
					Name:        state.Name,
					PID:         state.PID,
					GracePeriod: 10 * time.Second,
					ForceKill:   true,
				}).Get(ctx, &stopResult)

				if err != nil {
					logger.Error("StopMCPActivity failed during restart", "error", err)
				}
				state.PID = 0
			}

			// Start again
			state.State = MCPStateStarting
			var startResult StartMCPResult
			err := workflow.ExecuteActivity(activityCtx, "StartMCPActivity", StartMCPInput{
				Name:        config.Name,
				MCPCommand:  config.MCPCommand,
				Port:        config.Port,
				ServerPath:  config.ServerPath,
				Environment: config.Environment,
			}).Get(ctx, &startResult)

			if err != nil {
				logger.Error("StartMCPActivity failed during restart", "error", err)
				state.State = MCPStateStopped
				state.FailureCount++
			} else {
				logger.Info("MCP service restarted", "pid", startResult.PID)
				state.State = MCPStateRunning
				state.PID = startResult.PID
				state.StartedAt = startResult.StartedAt
				state.FailureCount = 0
			}
		})

		// Handle health check signal
		selector.AddReceive(healthChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				return
			}
			c.Receive(ctx, nil)

			if state.State == MCPStateRunning {
				logger.Info("Performing health check", "name", state.Name)

				// Execute health check activity with shorter timeout
				healthAO := workflow.ActivityOptions{
					StartToCloseTimeout: 10 * time.Second,
					RetryPolicy: &temporal.RetryPolicy{
						MaximumAttempts: 2,
					},
				}
				healthCtx := workflow.WithActivityOptions(ctx, healthAO)

				var healthResult HealthCheckResult
				err := workflow.ExecuteActivity(healthCtx, "HealthCheckActivity", HealthCheckInput{
					URL:     state.HealthCheckURL,
					Timeout: 5 * time.Second,
				}).Get(ctx, &healthResult)

				if err != nil {
					logger.Error("HealthCheckActivity failed", "error", err)
					state.FailureCount++

					// If too many failures, auto-restart
					if state.FailureCount >= 3 {
						logger.Warn("Too many health check failures, triggering restart", "failureCount", state.FailureCount)
						// Trigger restart by sending restart signal internally
						workflow.SignalExternalWorkflow(ctx, workflow.GetInfo(ctx).WorkflowExecution.ID, "", SignalRestartMCP, nil)
					}
				} else {
					logger.Info("Health check passed", "status", healthResult.Status, "uptime", healthResult.Uptime)
					state.LastHealthAt = workflow.Now(ctx)
					state.FailureCount = 0
				}
			}
		})

		// Periodic health check timer (every 30 seconds when running)
		if state.State == MCPStateRunning {
			timer := workflow.NewTimer(ctx, 30*time.Second)
			selector.AddFuture(timer, func(f workflow.Future) {
				// Trigger health check via signal
				workflow.SignalExternalWorkflow(ctx, workflow.GetInfo(ctx).WorkflowExecution.ID, "", SignalHealthMCP, nil)
			})
		}

		selector.Select(ctx)

		// Exit condition: stopped and received stop signal
		if state.State == MCPStateStopped {
			// Wait briefly for any restart signal, then exit
			restartReceived := false
			selector := workflow.NewSelector(ctx)

			timeout := workflow.NewTimer(ctx, 5*time.Second)
			selector.AddFuture(timeout, func(f workflow.Future) {
				// Timeout - exit workflow
			})

			selector.AddReceive(restartChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				restartReceived = true
			})

			selector.AddReceive(startChannel, func(c workflow.ReceiveChannel, more bool) {
				if !more {
					return
				}
				c.Receive(ctx, nil)
				restartReceived = true
			})

			selector.Select(ctx)

			if !restartReceived {
				break
			}
		}
	}

	logger.Info("MCPServiceWorkflow completed", "name", config.Name, "finalState", state.State)
	return nil
}

// StartMCPInput contains parameters for starting MCP service
type StartMCPInput struct {
	Name        string
	MCPCommand  string
	Port        int
	ServerPath  string
	Environment map[string]string
}

// StartMCPResult contains the result of starting MCP service
type StartMCPResult struct {
	PID       int
	Port      int
	StartedAt time.Time
	Command   string
}

// StopMCPInput contains parameters for stopping MCP service
type StopMCPInput struct {
	Name        string
	PID         int
	GracePeriod time.Duration
	ForceKill   bool
}

// StopMCPResult contains the result of stopping MCP service
type StopMCPResult struct {
	ProcessKilled bool
	GracefulExit  bool
	StoppedAt     time.Time
}

// HealthCheckInput contains parameters for health check
type HealthCheckInput struct {
	URL     string
	Timeout time.Duration
}

// HealthCheckResult contains the result of health check
type HealthCheckResult struct {
	Status       string // "healthy" or "unhealthy"
	Uptime       time.Duration
	SessionCount int
	Timestamp    time.Time
}
