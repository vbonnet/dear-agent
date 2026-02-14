# Temporal Activities for AGM

This package implements Temporal activities for managing agent processes in AGM sessions.

## Overview

Activities are the building blocks of Temporal workflows. Each activity performs a specific, idempotent operation that can be retried, monitored, and traced.

## Activities

### LaunchAgentActivity

Launches a Claude Code or Gemini CLI process.

**Input:**
- `SessionName`: Human-readable session name
- `SessionID`: Unique session identifier (UUID)
- `WorkDir`: Working directory for the agent
- `AgentType`: Type of agent ("claude" or "gemini")
- `Environment`: Additional environment variables

**Output:**
- `PID`: Process ID of launched agent
- `SessionID`: Session identifier
- `StartedAt`: Process start timestamp
- `Command`: Executed command

**Usage:**
```go
input := LaunchAgentInput{
    SessionName: "my-project",
    SessionID:   "abc-123",
    WorkDir:     "/home/user/projects/my-project",
    AgentType:   "claude",
    Environment: map[string]string{
        "ANTHROPIC_API_KEY": "sk-...",
    },
}

output, err := LaunchAgentActivity(ctx, input)
```

### MonitorOutputActivity

Monitors agent stdout/stderr for escalation patterns (errors, prompts, warnings).

**Input:**
- `SessionID`: Session to monitor
- `PID`: Process ID to monitor
- `Reader`: Output stream to monitor
- `Timeout`: Monitoring timeout duration
- `MaxLines`: Maximum lines to buffer

**Output:**
- `LinesRead`: Total lines processed
- `Escalations`: Detected escalation events
- `LastActivity`: Timestamp of last output
- `Completed`: Whether monitoring completed normally

**Escalation Patterns:**
- **Errors**: `error:`, `fatal:`, `failed to`, `permission denied`, `rate limit`, `authentication.*failed`
- **Prompts**: `(yes/no)`, `enter.*:`, `continue?`, `press.*key`
- **Warnings**: `warning:`, `deprecated:`

**Usage:**
```go
input := MonitorOutputInput{
    SessionID: sessionID,
    PID:       processID,
    Reader:    processStdout,
    Timeout:   30 * time.Second,
    MaxLines:  10000,
}

output, err := MonitorOutputActivity(ctx, input)
if len(output.Escalations) > 0 {
    // Handle escalations
    fmt.Println(FormatEscalations(output.Escalations))
}
```

### CheckpointStateActivity

Saves workflow state to persistent storage (JSON file in Phase 1, SQLite in Phase 2).

**Input:**
- `SessionID`: Session identifier
- `SessionName`: Session name
- `WorkflowID`: Temporal workflow ID
- `WorkflowRunID`: Temporal workflow run ID
- `State`: Workflow state (map)
- `Metadata`: Additional metadata
- `CheckpointType`: Type ("periodic", "manual", "before_escalation")

**Output:**
- `SessionID`: Session identifier
- `CheckpointPath`: Path to checkpoint file
- `CheckpointedAt`: Checkpoint timestamp
- `StateSize`: Size in bytes
- `Success`: Whether checkpoint succeeded

**Usage:**
```go
input := CheckpointStateInput{
    SessionID:      sessionID,
    SessionName:    "my-session",
    WorkflowID:     workflowID,
    WorkflowRunID:  runID,
    State: map[string]interface{}{
        "current_step": 3,
        "tasks_completed": 5,
        "status": "running",
    },
    CheckpointType: "periodic",
}

output, err := CheckpointStateActivity(ctx, input)
```

**Helper Functions:**
- `LoadCheckpointActivity(ctx, sessionID)`: Load latest checkpoint
- `ListCheckpointsActivity(ctx, sessionID)`: List all checkpoints (Phase 1: returns 1)
- `DeleteCheckpointActivity(ctx, sessionID)`: Delete checkpoint
- `SaveWorkflowState(sessionID, key, value)`: Save single state value
- `GetWorkflowState(sessionID, key)`: Retrieve single state value

### TerminateSessionActivity

Gracefully terminates agent process and cleans up resources.

**Input:**
- `SessionID`: Session to terminate
- `SessionName`: Session name
- `PID`: Process ID to terminate
- `GracePeriod`: Grace period for graceful shutdown (default: 10s)
- `ForceKill`: Whether to force kill if graceful fails
- `CleanupFiles`: Whether to cleanup temporary files
- `ArchiveSession`: Whether to archive session data

**Output:**
- `ProcessKilled`: Whether process was killed
- `FilesRemoved`: Number of temporary files removed
- `SessionArchived`: Whether session was archived
- `TerminatedAt`: Termination timestamp
- `GracefulExit`: Whether process exited gracefully

**Termination Process:**
1. Send SIGTERM for graceful shutdown
2. Wait for grace period
3. Send SIGKILL if needed (if `ForceKill=true`)
4. Cleanup temporary files (if `CleanupFiles=true`)
5. Archive session data (if `ArchiveSession=true`)

**Usage:**
```go
input := TerminateSessionInput{
    SessionID:      sessionID,
    SessionName:    "my-session",
    PID:            processID,
    GracePeriod:    10 * time.Second,
    ForceKill:      true,
    CleanupFiles:   true,
    ArchiveSession: true,
}

output, err := TerminateSessionActivity(ctx, input)
```

**Helper Functions:**
- `KillProcessActivity(ctx, pid)`: Force kill a process
- `CheckProcessActivity(ctx, pid)`: Check if process is running
- `CleanupSessionActivity(ctx, sessionID)`: Remove all session data

## Session Directory Structure

Activities use the following directory structure:

```
~/.agm/
├── sessions/           # Active session data
│   └── <session-id>/
│       ├── checkpoint.json     # Workflow checkpoint
│       ├── manifest.yaml       # Session manifest
│       ├── session.log         # Session logs
│       └── *.tmp               # Temporary files
└── archive/            # Archived sessions
    └── <session-id>/
        ├── checkpoint.json
        ├── manifest.yaml
        ├── session.log
        └── archive_info.txt
```

## Helper Functions

### GetSessionDataDir(sessionID)

Returns the data directory path for a session:
```
~/.agm/sessions/<session-id>
```

### EnsureSessionDir(sessionID)

Creates the session directory if it doesn't exist. Returns the directory path.

## Error Handling

All activities follow Temporal best practices:

1. **Validation**: Input parameters are validated early
2. **Idempotency**: Activities can be safely retried
3. **Error Types**: Clear error messages with context
4. **Cleanup**: Resources are cleaned up on failure
5. **Logging**: Operations are logged for debugging

## Testing

Run tests with:
```bash
go test ./internal/temporal/activities/... -v -cover
```

Test coverage goal: 80%+

### Test Categories

1. **Validation Tests**: Empty/invalid inputs
2. **Success Cases**: Normal operation paths
3. **Error Cases**: Failure scenarios
4. **Edge Cases**: Boundary conditions, timeouts
5. **Integration Tests**: Multi-step operations

## Phase 2 Enhancements

Future improvements planned:

1. **SQLite Backend**: Migrate checkpoint storage from JSON to SQLite
2. **Advanced Monitoring**: More sophisticated output pattern matching
3. **Process Groups**: Support for multi-process agents
4. **Resource Limits**: CPU/memory monitoring and limits
5. **Metrics**: Prometheus metrics for activity execution
6. **Distributed Tracing**: OpenTelemetry integration

## Dependencies

- `os/exec`: Process management
- `context`: Cancellation and timeouts
- `encoding/json`: Checkpoint serialization
- `regexp`: Pattern matching for escalations
- `syscall`: Signal handling

## Usage in Workflows

Activities are called from Temporal workflows:

```go
func AgentSessionWorkflow(ctx workflow.Context, input WorkflowInput) error {
    // Launch agent
    launchInput := LaunchAgentInput{...}
    var launchOutput LaunchAgentOutput
    err := workflow.ExecuteActivity(ctx, LaunchAgentActivity, launchInput).Get(ctx, &launchOutput)

    // Monitor output
    monitorInput := MonitorOutputInput{...}
    var monitorOutput MonitorOutputOutput
    err = workflow.ExecuteActivity(ctx, MonitorOutputActivity, monitorInput).Get(ctx, &monitorOutput)

    // Checkpoint state
    checkpointInput := CheckpointStateInput{...}
    var checkpointOutput CheckpointStateOutput
    err = workflow.ExecuteActivity(ctx, CheckpointStateActivity, checkpointInput).Get(ctx, &checkpointOutput)

    // Terminate session
    terminateInput := TerminateSessionInput{...}
    var terminateOutput TerminateSessionOutput
    err = workflow.ExecuteActivity(ctx, TerminateSessionActivity, terminateInput).Get(ctx, &terminateOutput)

    return nil
}
```

## See Also

- [Temporal Documentation](https://docs.temporal.io/)
- [AGM Architecture](../../../../docs/ARCHITECTURE.md)
- [Workflow Documentation](../README.md)
