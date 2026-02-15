# MCP Service Workflow

## Overview

The MCP Service Workflow manages the lifecycle of global MCP HTTP server processes using Temporal. This workflow provides reliable process management with health monitoring, automatic restarts, and graceful shutdown capabilities.

## Architecture

### Workflow: `MCPServiceWorkflow`

**File**: `mcp_service_workflow.go`

Manages the complete lifecycle of an MCP HTTP server process:

- **States**: `stopped` → `starting` → `running` → `stopping` → `stopped`
- **Signals**: `StartMCP`, `StopMCP`, `RestartMCP`, `HealthCheck`
- **Query**: `GetMCPState` - retrieves current workflow state
- **Auto-recovery**: Periodic health checks with automatic restart on failure

### Activities

#### 1. StartMCPActivity

**File**: `activities/start_mcp_activity.go`

Starts the MCP HTTP server process.

**Command executed**:
```bash
node <server.js> --mcp-command "<mcp-command>" --port <port>
```

**Example**:
```bash
node ~/src/ws/oss/swarm/projects/mcp-global-sharing/packages/mcp-http-server/dist/server.js \
  --mcp-command "npx -y @modelcontextprotocol/server-googledocs" \
  --port 8001
```

**Responsibilities**:
- Validates input parameters
- Creates MCP data directory (`~/.agm/mcp-services/<name>`)
- Redirects stdout/stderr to log files
- Stores PID for later management
- Verifies process started successfully

**Returns**: PID, port, startup timestamp

#### 2. StopMCPActivity

**File**: `activities/stop_mcp_activity.go`

Gracefully stops the MCP HTTP server.

**Process**:
1. Send SIGTERM for graceful shutdown
2. Wait for grace period (default 10s)
3. Force kill with SIGKILL if timeout
4. Clean up PID file

**Returns**: Process killed status, graceful exit flag

#### 3. HealthCheckActivity

**File**: `activities/health_check_activity.go`

Performs HTTP health check on the MCP server.

**Endpoint**: `GET http://localhost:<port>/health`

**Expected response**:
```json
{
  "status": "healthy",
  "uptime": 3600,
  "sessions": 2,
  "mcpProcess": "running"
}
```

**Timeout**: 5 seconds (configurable)

**Retry**: 2 attempts on transient failures

**Returns**: Health status, uptime, session count

## Configuration

```go
config := workflows.MCPServiceConfig{
    Name:           "googledocs",
    MCPCommand:     "npx -y @modelcontextprotocol/server-googledocs",
    Port:           8001,
    ServerPath:     "/path/to/server.js", // Optional, defaults to standard location
    Environment:    map[string]string{},  // Optional environment variables
    HealthCheckURL: "http://localhost:8001/health", // Optional, auto-generated
}
```

## Usage

### Starting a Workflow

```go
import (
    "go.temporal.io/sdk/client"
    "github.com/vbonnet/claude-session-manager/internal/temporal/workflows"
)

// Create Temporal client
temporalClient, err := client.Dial(client.Options{
    HostPort: "localhost:7233",
})

// Configure MCP service
config := workflows.MCPServiceConfig{
    Name:       "googledocs",
    MCPCommand: "npx -y @modelcontextprotocol/server-googledocs",
    Port:       8001,
}

// Start workflow
workflowOptions := client.StartWorkflowOptions{
    ID:        "mcp-service-googledocs",
    TaskQueue: "mcp-service-queue",
}

workflowRun, err := temporalClient.ExecuteWorkflow(
    context.Background(),
    workflowOptions,
    workflows.MCPServiceWorkflow,
    config,
)
```

### Sending Signals

```go
// Start the MCP server
err = temporalClient.SignalWorkflow(
    context.Background(),
    "mcp-service-googledocs",
    "",
    workflows.SignalStartMCP,
    nil,
)

// Perform health check
err = temporalClient.SignalWorkflow(
    context.Background(),
    "mcp-service-googledocs",
    "",
    workflows.SignalHealthMCP,
    nil,
)

// Restart the MCP server
err = temporalClient.SignalWorkflow(
    context.Background(),
    "mcp-service-googledocs",
    "",
    workflows.SignalRestartMCP,
    nil,
)

// Stop the MCP server
err = temporalClient.SignalWorkflow(
    context.Background(),
    "mcp-service-googledocs",
    "",
    workflows.SignalStopMCP,
    nil,
)
```

### Querying State

```go
// Query current workflow state
resp, err := temporalClient.QueryWorkflow(
    context.Background(),
    "mcp-service-googledocs",
    "",
    workflows.QueryMCPState,
)

var state workflows.MCPServiceWorkflowState
err = resp.Get(&state)

fmt.Printf("State: %s, PID: %d, Port: %d\n", state.State, state.PID, state.Port)
```

## State Transitions

```
stopped
   |
   | (StartMCP signal)
   v
starting ----[StartMCPActivity]----> running
   |                                    |
   | (failure)                          | (StopMCP signal)
   v                                    v
stopped <------[StopMCPActivity]---- stopping
                                        |
                                        v
                                     stopped

running ----[RestartMCP signal]----> stopping -> starting -> running
   |
   | (health check failure x3)
   v
auto-restart
```

## Health Monitoring

The workflow performs automatic health checks every 30 seconds when the MCP server is running:

1. Health check signal sent automatically
2. HealthCheckActivity executed
3. On success: Reset failure counter
4. On failure: Increment failure counter
5. After 3 consecutive failures: Trigger automatic restart

## File Locations

### Data Directory
```
~/.agm/mcp-services/<service-name>/
├── mcp-server.log      # stdout logs
├── mcp-server.err      # stderr logs
└── mcp-server.pid      # process ID
```

### Server Script
```
~/src/ws/oss/swarm/projects/mcp-global-sharing/packages/mcp-http-server/dist/server.js
```

## Testing

### Integration Tests

**File**: `test/integration/mcp_service_workflow_test.go`

Tests include:
- Basic start/stop lifecycle
- Service restart
- State queries
- Health checks
- Input validation

**Run tests**:
```bash
cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager
go test -v ./internal/temporal/test/integration -run TestMCPServiceWorkflowTestSuite
```

**Prerequisites**:
- Temporal server running on `localhost:7233`
- MCP HTTP server built and available
- Node.js installed

## Error Handling

### Activity Retry Policies

All activities use the following retry policy:
- Maximum attempts: 3
- Timeout: 30 seconds
- Exponential backoff

### Common Errors

1. **Server script not found**
   - Check `ServerPath` configuration
   - Verify file exists and is executable

2. **Port already in use**
   - Choose different port
   - Stop existing process on that port

3. **Health check failures**
   - Server may not be fully started (allow more time)
   - Check server logs in data directory
   - Verify network connectivity

4. **Process won't stop**
   - Workflow uses force kill after grace period
   - Check for zombie processes
   - Verify PID file accuracy

## Design Patterns

This implementation follows the SessionWorkflow pattern from the codebase:

1. **State machine**: Explicit state transitions with validation
2. **Signal-driven**: Event-based control flow
3. **Query handlers**: Real-time state inspection
4. **Activity separation**: Clear boundaries between workflow logic and external operations
5. **Graceful degradation**: Automatic recovery and fallback mechanisms

## Future Enhancements

- [ ] Multi-instance support (multiple MCP servers per workflow)
- [ ] Metrics collection and monitoring
- [ ] Log rotation and archival
- [ ] Dynamic configuration updates
- [ ] Circuit breaker pattern for health checks
- [ ] Integration with existing session management
