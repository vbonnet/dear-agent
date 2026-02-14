// Package activities implements Temporal activities for AGM agent process management.
//
// This package provides four core activities that enable Temporal-based session management:
//
// 1. LaunchAgentActivity: Spawns Claude Code or Gemini CLI processes
// 2. MonitorOutputActivity: Detects escalations in agent output (errors, prompts)
// 3. CheckpointStateActivity: Persists workflow state for recovery
// 4. TerminateSessionActivity: Gracefully terminates processes and cleans up resources
//
// Activities are designed to be idempotent, retriable, and composable within Temporal workflows.
// They follow Temporal best practices for error handling, timeouts, and state management.
//
// Example workflow usage:
//
//	func AgentSessionWorkflow(ctx workflow.Context, input WorkflowInput) error {
//	    // Launch agent process
//	    var launchOutput LaunchAgentOutput
//	    err := workflow.ExecuteActivity(ctx, LaunchAgentActivity, LaunchAgentInput{
//	        SessionName: "my-session",
//	        WorkDir:     "/home/user/project",
//	        AgentType:   "claude",
//	    }).Get(ctx, &launchOutput)
//	    if err != nil {
//	        return err
//	    }
//
//	    // Monitor for escalations
//	    var monitorOutput MonitorOutputOutput
//	    err = workflow.ExecuteActivity(ctx, MonitorOutputActivity, MonitorOutputInput{
//	        SessionID: launchOutput.SessionID,
//	        PID:       launchOutput.PID,
//	        Timeout:   30 * time.Second,
//	    }).Get(ctx, &monitorOutput)
//
//	    // Handle escalations...
//	    return nil
//	}
//
// Phase 1 Implementation:
//   - JSON-based checkpoint storage (Phase 2 will migrate to SQLite)
//   - Basic escalation pattern matching
//   - Process lifecycle management
//
// See README.md for detailed documentation on each activity.
package activities
