# Temporal Workflows

This package implements the core Temporal workflows for managing AGM sessions in a distributed, fault-tolerant manner.

## Overview

The workflows package provides three main workflows:

1. **SessionWorkflow** - Manages session lifecycle (create → active → stopped → archived)
2. **MonitorWorkflow** - Monitors agent output for escalation patterns
3. **EscalationWorkflow** - Handles escalation notifications

## SessionWorkflow

Manages the complete lifecycle of an AGM session.

### States

- `created` - Initial state after session creation
- `active` - Session is running and accepting commands
- `stopped` - Session is paused/stopped but not archived
- `archived` - Session is permanently archived (terminal state)

### Signals

- `activate` - Transition session to active state
- `stop` - Transition session to stopped state
- `archive` - Archive session and complete workflow
- `attach` - Increment attached client counter
- `detach` - Decrement attached client counter

### Queries

- `getSessionState` - Returns current session state

### Activities Used

- `CreateSessionActivity` - Creates the session resources
- `ActivateSessionActivity` - Activates a stopped session
- `StopSessionActivity` - Stops an active session
- `ArchiveSessionActivity` - Archives the session

### Example Usage

```go
input := SessionWorkflowInput{
    SessionID:   "session-123",
    SessionName: "my-session",
    WorkingDir:  "/home/user/project",
    Agent:       "claude",
    Project:     "/home/user/project",
    Tags:        []string{"dev", "feature-x"},
}

workflowID := "session-session-123"
workflowRun, err := client.ExecuteWorkflow(ctx,
    client.StartWorkflowOptions{
        ID:        workflowID,
        TaskQueue: "agm-sessions",
    },
    SessionWorkflow,
    input,
)
```

## MonitorWorkflow

Continuously monitors agent output (stdout/stderr) for configured escalation patterns.

### Features

- Periodic output checking (configurable interval)
- Pattern matching against escalation rules
- Threshold-based escalation triggering
- Child workflow spawning for escalations
- Dynamic rule updates via signals

### Signals

- `startMonitoring` - Start/resume monitoring
- `stopMonitoring` - Pause/stop monitoring
- `updateRules` - Update escalation rules dynamically

### Queries

- `getMonitorState` - Returns current monitoring state

### Activities Used

- `FetchSessionOutputActivity` - Retrieves session output

### Example Usage

```go
input := MonitorWorkflowInput{
    SessionID:       "session-123",
    MonitorInterval: 5 * time.Second,
    EscalationRules: []EscalationRule{
        {
            Name:        "Error Pattern",
            Patterns:    []string{"ERROR", "FATAL"},
            Severity:    "high",
            NotifyAfter: 2, // Trigger after 2 matches
        },
        {
            Name:        "Warning Pattern",
            Patterns:    []string{"WARN", "WARNING"},
            Severity:    "medium",
            NotifyAfter: 5,
        },
    },
}

workflowRun, err := client.ExecuteWorkflow(ctx,
    client.StartWorkflowOptions{
        ID:        "monitor-session-123",
        TaskQueue: "agm-monitors",
    },
    MonitorWorkflow,
    input,
)
```

## EscalationWorkflow

Handles notification delivery when escalation patterns are detected.

### Features

- Multi-channel notification support (email, Slack, webhook, log)
- Severity-based retry policies
- Critical severity fallback handling
- Notification result tracking

### Severity Levels

- `critical` - 5 retry attempts, 1s initial interval, requires fallback
- `high` - 3 retry attempts, 2s initial interval
- `medium` - 2 retry attempts, 5s initial interval
- `low` - 2 retry attempts, 5s initial interval

### Queries

- `getEscalationState` - Returns current escalation state

### Activities Used

- `LogEscalationActivity` - Logs the escalation event
- `SendNotificationActivity` - Sends notification to a channel
- `StoreEscalationRecordActivity` - Stores escalation record for audit

### Example Usage

```go
input := EscalationWorkflowInput{
    SessionID:   "session-123",
    RuleName:    "Error Pattern",
    Severity:    "high",
    MatchedText: "ERROR: Database connection failed",
    Timestamp:   time.Now(),
    NotificationChannels: []NotificationChannel{
        {
            Type:     "slack",
            Target:   "#alerts",
            Priority: 0,
        },
        {
            Type:     "email",
            Target:   "team@example.com",
            Priority: 1,
        },
    },
}

workflowRun, err := client.ExecuteWorkflow(ctx,
    client.StartWorkflowOptions{
        ID:        "escalation-session-123-error-001",
        TaskQueue: "agm-escalations",
    },
    EscalationWorkflow,
    input,
)
```

## Testing

### Integration Tests

Run integration tests with Temporal test server:

```bash
cd internal/temporal/workflows
go test -v
```

### BDD Tests

Run BDD tests using Godog:

```bash
cd test/bdd
godog features/temporal_workflows.feature
```

### Coverage

To achieve 85%+ coverage:

```bash
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Workflow Patterns

### Parent-Child Workflow

MonitorWorkflow spawns EscalationWorkflow as a child workflow when patterns are detected:

```go
cwo := workflow.ChildWorkflowOptions{
    WorkflowID: fmt.Sprintf("escalation-%s-%s-%d",
        sessionID, ruleName, timestamp),
}
childCtx := workflow.WithChildOptions(ctx, cwo)

var result string
err := workflow.ExecuteChildWorkflow(childCtx,
    EscalationWorkflow,
    escalationInput,
).Get(ctx, &result)
```

### Signal-Query Pattern

All workflows support signal-query pattern for state management:

```go
// Send signal
err := client.SignalWorkflow(ctx, workflowID, "", SignalStop, nil)

// Query state
resp, err := client.QueryWorkflow(ctx, workflowID, "", QuerySessionState)
var state SessionWorkflowState
resp.Get(&state)
```

### Activity Retry

Activities are configured with retry policies:

```go
ao := workflow.ActivityOptions{
    StartToCloseTimeout: 30 * time.Second,
    RetryPolicy: &workflow.RetryPolicy{
        MaximumAttempts: 3,
    },
}
ctx = workflow.WithActivityOptions(ctx, ao)
```

## Architecture Notes

### State Management

- Workflows maintain state in-memory
- State is queryable via query handlers
- State is persisted automatically by Temporal

### Fault Tolerance

- Workflows are deterministic and replayable
- Activities can fail and retry automatically
- Workflows can be stopped and resumed
- Child workflows are tracked and managed

### Scalability

- Multiple workflows can run concurrently
- Each session has its own workflow instance
- Monitor workflows poll independently
- Escalation workflows are short-lived

## Dependencies

- `go.temporal.io/sdk v1.28.1` - Temporal SDK for Go
- Activities implementation in `internal/temporal/activities/` (Task 1.3)

## Next Steps

Task 1.3 will implement the activity functions referenced by these workflows:

- CreateSessionActivity
- ActivateSessionActivity
- StopSessionActivity
- ArchiveSessionActivity
- FetchSessionOutputActivity
- LogEscalationActivity
- SendNotificationActivity
- StoreEscalationRecordActivity
