# Global MCP Integration for AGM

This package implements integration between AGM (agm) and global MCP servers.

## Overview

Global MCPs are HTTP/SSE MCP servers that can be shared across multiple AGM sessions. Instead of spawning a new stdio MCP process for each session, AGM can connect to a global MCP server that's already running.

### Benefits

1. **Resource Efficiency**: One MCP process serves multiple sessions
2. **Faster Startup**: No need to spawn new processes per session
3. **Shared State**: MCPs can maintain state across sessions (e.g., authentication)
4. **Better Monitoring**: Centralized health checks and metrics

## Architecture

### Components

1. **GlobalMCPDetector** (`detector.go`)
   - Detects available global MCPs via HTTP health checks
   - Validates MCP availability before connection

2. **HTTPClient** (`http_client.go`)
   - Connects to global MCPs via HTTP/SSE
   - Implements JSON-RPC protocol over HTTP
   - Manages session lifecycle

3. **MCPManager** (`manager.go`)
   - Manages MCP connections for AGM sessions
   - Merges global and session-specific MCP configs
   - Reference counting for shared global MCPs

4. **SessionMCPIntegration** (`integration.go`)
   - Integrates MCP management into AGM session lifecycle
   - Provides Claude CLI arguments with MCP configuration
   - Handles cleanup without killing global MCPs

### Flow

```
AGM Session Start
    ↓
Load Global MCP Config (~/.config/agm/mcp.yaml)
    ↓
Load Session MCP Config (.agm/mcp.yaml)
    ↓
Detect Available Global MCPs (HTTP health check)
    ↓
┌─────────────────────────────────────┐
│ Global MCP Available?               │
│  YES → Connect via HTTP/SSE         │
│  NO  → Fallback to stdio MCP        │
└─────────────────────────────────────┘
    ↓
Start Claude with MCP configuration
    ↓
Session runs...
    ↓
Session End
    ↓
┌─────────────────────────────────────┐
│ Disconnect from MCPs                │
│  Global MCPs: Close session only    │
│  Session MCPs: Terminate process    │
└─────────────────────────────────────┘
```

## Configuration

### Global MCP Configuration

File: `~/.config/agm/mcp.yaml`

```yaml
mcp_servers:
  - name: googledocs
    url: http://localhost:8001
    type: mcp

  - name: github
    url: http://localhost:8002
    type: mcp
```

### Session-Specific MCP Configuration

File: `<project>/.agm/mcp.yaml`

```yaml
mcp_servers:
  - name: local-tools
    command: node
    args:
      - /path/to/local-mcp-server.js
    env:
      DEBUG: "true"
```

### Environment Variable Configuration

Alternative to YAML config:

```bash
export AGM_MCP_SERVERS="googledocs=http://localhost:8001,github=http://localhost:8002"
```

## Usage

### Check Global MCP Status

```bash
agm mcp-status
```

Output:
```
Global MCP Server Status:

NAME           STATUS        URL                                      ERROR
----           ------        ---                                      -----
googledocs     AVAILABLE     http://localhost:8001
github         UNAVAILABLE   http://localhost:8002                    connection refused

Summary: 1/2 global MCPs available
```

### Start AGM Session with Global MCPs

```bash
# Global MCPs are detected automatically
agm session new my-session

# Check which MCPs are connected
# (future: agm session info my-session)
```

### Programmatic Usage

```go
package main

import (
    "context"
    "github.com/vbonnet/dear-agent/agm/internal/mcp"
)

func main() {
    // Create MCP integration
    integration := mcp.NewSessionMCPIntegration("my-session", "/path/to/project")

    // Initialize (load configs)
    ctx := context.Background()
    if err := integration.Initialize(ctx); err != nil {
        panic(err)
    }

    // Connect to MCP
    conn, err := integration.ConnectToMCP(ctx, "googledocs")
    if err != nil {
        panic(err)
    }

    // Use MCP...

    // Cleanup (doesn't kill global MCPs)
    if err := integration.Cleanup(ctx); err != nil {
        panic(err)
    }
}
```

## API Reference

### GlobalMCPDetector

```go
type GlobalMCPDetector struct {
    healthCheckTimeout time.Duration
}

// Create detector with default 3s timeout
detector := mcp.NewGlobalMCPDetector()

// Detect single MCP
result := detector.DetectGlobalMCP(ctx, serverConfig)
if result.Available {
    fmt.Println("MCP is available")
}

// Detect all configured MCPs
results := detector.DetectAllGlobalMCPs(ctx, config)
```

### HTTPClient

```go
// Create HTTP client
client := mcp.NewHTTPClient("http://localhost:8001")

// Create session
err := client.CreateSession(ctx, "client-id")

// Send request
response, err := client.SendRequest(ctx, "tools/list", nil)

// Close session (doesn't kill server)
err = client.CloseSession(ctx)
```

### MCPManager

```go
// Create manager
manager := mcp.NewMCPManager()

// Load configs
manager.LoadGlobalConfig()
manager.LoadSessionConfig("/path/to/project")

// Connect to MCP (auto-detects global vs session)
conn, err := manager.ConnectToMCP(ctx, "googledocs")

// Disconnect (doesn't kill global MCPs)
err = manager.DisconnectMCP(ctx, "googledocs")

// Disconnect all
err = manager.DisconnectAll(ctx)
```

## Integration with Temporal Workflow

Global MCPs are managed by Temporal workflows for lifecycle management:

```go
// internal/temporal/workflows/mcp_service_workflow.go
config := workflows.MCPServiceConfig{
    Name:       "googledocs",
    MCPCommand: "npx -y @modelcontextprotocol/server-googledocs",
    Port:       8001,
    ServerPath: "/path/to/mcp-http-server/dist/server.js",
}

// Start workflow
workflowID := "mcp-service-googledocs"
err := temporalClient.ExecuteWorkflow(ctx, workflowOptions,
    workflows.MCPServiceWorkflow, config)
```

The workflow:
1. Starts the MCP HTTP server
2. Monitors health via periodic checks
3. Auto-restarts on failures
4. Stops gracefully on shutdown

## Session Lifecycle

### Session Start

1. Load global MCP config (`~/.config/agm/mcp.yaml`)
2. Load session MCP config (`<project>/.agm/mcp.yaml`)
3. Detect available global MCPs (HTTP health check)
4. Connect to global MCPs (HTTP/SSE) or fallback to stdio
5. Set environment variables for MCP integration
6. Start Claude with MCP configuration

### Session End

1. Close HTTP/SSE sessions for global MCPs
2. **DO NOT** kill global MCP processes
3. Terminate session-specific MCP processes
4. Clean up temporary files

## Testing

### Unit Tests

```bash
cd main/agm
go test ./internal/mcp/...
```

### Integration Tests

```bash
# Start test HTTP server
cd packages/mcp-http-server
npm run dev -- --port 8001 --mcp-command "npx -y @modelcontextprotocol/server-googledocs"

# Run AGM with global MCP
agm mcp-status
agm session new test-session
```

## Debugging

Enable debug logging:

```bash
export AGM_DEBUG=true
agm session new my-session
```

Check MCP health:

```bash
# Manual health check
curl http://localhost:8001/health

# AGM health check
agm mcp-status
```

View MCP logs:

```bash
# Global MCP logs (Temporal workflow)
tail -f ~/.agm/mcp-services/googledocs/mcp-server.log

# Session-specific MCP logs
tail -f <project>/.agm/mcp-session.log
```

## Migration Guide

### From stdio MCPs to Global MCPs

1. **Identify MCPs to convert**:
   - Look at current Claude config (`~/.claude/config.json`)
   - Identify frequently used MCPs

2. **Start global MCP servers**:
   ```bash
   # Example: Google Docs MCP
   cd packages/mcp-http-server
   npm run dev -- --port 8001 --mcp-command "npx -y @modelcontextprotocol/server-googledocs"
   ```

3. **Configure global MCPs**:
   ```yaml
   # ~/.config/agm/mcp.yaml
   mcp_servers:
     - name: googledocs
       url: http://localhost:8001
       type: mcp
   ```

4. **Verify detection**:
   ```bash
   agm mcp-status
   ```

5. **Test with new session**:
   ```bash
   agm session new test-global-mcp
   ```

## Future Enhancements

1. **Auto-start global MCPs**: Automatically start global MCPs on daemon startup
2. **MCP registry**: Discover MCPs via service registry (Consul, etcd)
3. **Load balancing**: Distribute sessions across multiple MCP instances
4. **Authentication**: Support authenticated MCP connections
5. **Metrics**: Expose Prometheus metrics for MCP usage
6. **WebSocket support**: Add WebSocket transport alongside HTTP/SSE

## Troubleshooting

### Global MCP not detected

```bash
# Check if server is running
curl http://localhost:8001/health

# Check firewall
sudo iptables -L | grep 8001

# Check port binding
netstat -tuln | grep 8001
```

### Connection timeouts

```bash
# Increase timeout in config
export AGM_MCP_TIMEOUT=10s

# Check network latency
ping localhost
```

### Session hangs on startup

```bash
# Enable debug mode
export AGM_DEBUG=true

# Check Claude logs
tail -f ~/.agm/sessions/*/claude.log

# Check MCP logs
tail -f ~/.agm/mcp-services/*/mcp-server.log
```

## References

- [MCP HTTP Server](packages/mcp-http-server/README.md)
- [Temporal Workflow](main/agm/internal/temporal/workflows/mcp_service_workflow.go)
- [MCP Specification](https://spec.modelcontextprotocol.io/)
