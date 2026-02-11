# ADR 005: Stdio Transport Logging Strategy

## Status

Accepted

## Context

The AGM MCP server uses stdio transport, which means:
- **stdin**: Receives JSON-RPC requests from MCP client
- **stdout**: Sends JSON-RPC responses to MCP client
- **stderr**: Available for logging

Writing logs to stdout would corrupt the JSON-RPC protocol, causing client failures. However, the server needs logging for:
- Startup diagnostics (version, config)
- Error reporting (config failures, disk errors)
- Performance monitoring (query latency)
- Debugging (request tracing)

### Requirements

1. **Never write to stdout** (except JSON-RPC responses)
2. **Provide diagnostic information** for debugging
3. **Support production deployment** (minimal noise)
4. **Enable performance monitoring** (latency tracking)

### Options Considered

#### Option 1: No Logging

**Implementation**:
```go
// No log.Printf() calls
// Silently fail on errors
```

**Pros**:
- Zero risk of stdout corruption
- Simplest implementation
- No log noise

**Cons**:
- **Impossible to Debug**: No visibility into errors
- **Silent Failures**: Config errors go unnoticed
- **No Monitoring**: Cannot track performance
- **Poor DX**: Developers flying blind

#### Option 2: File-Based Logging

**Implementation**:
```go
logFile, _ := os.OpenFile("/var/log/agm-mcp-server.log", ...)
log.SetOutput(logFile)
```

**Pros**:
- Persistent logs (can review after process exits)
- No stdout corruption
- Structured logging possible (JSON)

**Cons**:
- **File Path Management**: Where to put logs? User-writable?
- **Permission Issues**: What if log dir not writable?
- **Log Rotation**: Who rotates logs? Disk space management?
- **Complexity**: Extra configuration (log file path in config)
- **V1 Scope Creep**: Adds significant complexity

#### Option 3: Stderr Logging

**Implementation**:
```go
log.SetOutput(os.Stderr)
log.Printf("Starting AGM MCP Server v1.0.0")
```

**Pros**:
- **No Stdout Corruption**: stderr is separate from stdout
- **Standard Practice**: Logs go to stderr by convention
- **Simple Implementation**: Just redirect default logger
- **No File Management**: No log files, no rotation
- **IDE/Terminal Integration**: stderr visible in terminal/logs

**Cons**:
- Ephemeral (logs lost when process exits)
- No log aggregation (unless parent captures stderr)
- Limited structure (text only, no JSON by default)

#### Option 4: External Logging Service

**Implementation**:
```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
logger.Info("startup", "version", Version)
```

**Pros**:
- Structured logging (JSON, key-value pairs)
- Log levels (debug, info, warn, error)
- Modern Go pattern (slog from Go 1.21)

**Cons**:
- **Dependency**: Requires Go 1.21+ or external library
- **V1 Scope**: Structured logging is nice-to-have, not required
- **Complexity**: More code than simple log.Printf()

## Decision

We will use **Option 3: Stderr Logging** with Go's standard `log` package.

## Rationale

### Stdio Protocol Requirement

The MCP stdio transport specification requires:
> JSON-RPC messages are written to stdout. Logs and diagnostics MUST be written to stderr.

This makes stderr the only viable option for logging.

### Simplicity for V1

V1 goals:
- Get MCP server working
- Validate use cases
- Gather feedback

Structured logging can wait for V2. Simple `log.Printf()` is sufficient.

### Standard Practice

Unix convention:
- stdout = program output (JSON-RPC responses)
- stderr = diagnostic messages (logs, errors)

Following this convention makes the server predictable.

### Claude Code Integration

When Claude Code launches the MCP server, it captures stderr for debugging. Users can view MCP server logs in Claude Code's developer console.

## Implementation

### Logger Setup

```go
// main.go
func main() {
    // CRITICAL: Redirect logs to stderr
    log.SetOutput(os.Stderr)

    // Print startup header
    fmt.Fprintf(os.Stderr, "agm-mcp-server %s (%s)\n", Version, executable)
}
```

### Log Levels (Informal)

Since we're using `log` package (no levels), use prefixes:
- `log.Printf("Starting...")` → Info
- `log.Printf("ERROR: ...")` → Error
- `log.Fatalf("...")` → Fatal (exit process)

### What to Log

#### Startup (Always)
```go
log.Printf("Starting AGM MCP Server v%s", Version)
log.Printf("Sessions directory: %s", cfg.SessionsDir)
log.Printf("Registered %d tools: %s", len(tools), toolNames)
log.Printf("Starting MCP server with stdio transport")
```

#### Errors (Always)
```go
log.Fatalf("Config load failed: %v", err)
log.Printf("ERROR: Failed to list sessions: %v", err)
```

#### Warnings (V2)
```go
log.Printf("WARN: Auto-registration failed (non-fatal): %v", err)
```

#### Debug (V2, env-gated)
```go
if os.Getenv("AGM_MCP_DEBUG") == "1" {
    log.Printf("DEBUG: Cache hit (age: %s)", time.Since(cacheTimestamp))
}
```

### What NOT to Log

- **MCP Requests**: Don't log every JSON-RPC request (noisy, privacy risk)
- **Session Data**: Don't log session names/IDs (privacy)
- **Performance Metrics**: Don't log every query latency (V1 scope)

## Log Format

### Standard Format

```
[timestamp] [severity] message
2025-01-15 10:30:45 Starting AGM MCP Server v1.0.0
2025-01-15 10:30:45 Sessions directory: ~/.config/agm/sessions
2025-01-15 10:30:45 Registered 3 tools: agm_list_sessions, agm_search_sessions, agm_get_session_metadata
```

Go's default logger includes timestamp automatically:
```go
log.Printf("message")
// Output: 2025/01/15 10:30:45 message
```

### Error Format

```
2025-01-15 10:30:45 ERROR: Failed to list sessions: permission denied
2025-01-15 10:30:45 WARN: Auto-registration skipped (config not writable)
```

## Claude Code Integration

### How Claude Code Captures Logs

When Claude Code launches the MCP server:
```bash
agm-mcp-server 2> /tmp/claude-code-mcp-agm.log
```

Claude Code captures stderr and displays it in:
- Developer Console (View → Toggle Developer Tools)
- MCP Server Logs panel

### User Access to Logs

Users can view logs:
1. Open Claude Code
2. Open Developer Tools (Cmd+Opt+I / Ctrl+Shift+I)
3. Navigate to MCP → agm → Logs

Or via terminal:
```bash
tail -f /tmp/claude-code-mcp-agm.log
```

## Performance Impact

### Logging Overhead

- `log.Printf()` to stderr: ~1µs per call
- Negligible impact for startup logs (10-20 lines)
- Avoid logging in hot path (cache hit on every query)

### Debug Logging

For V2, add env-gated debug logs:
```go
func logDebug(format string, args ...interface{}) {
    if os.Getenv("AGM_MCP_DEBUG") == "1" {
        log.Printf("DEBUG: "+format, args...)
    }
}
```

Usage:
```bash
AGM_MCP_DEBUG=1 agm-mcp-server
```

## Consequences

### Positive

- **No Stdout Corruption**: Logs never interfere with JSON-RPC
- **Simple Implementation**: Just `log.SetOutput(os.Stderr)`
- **Standard Practice**: Follows Unix conventions
- **Claude Code Integration**: Logs visible in developer console
- **Zero Dependencies**: Uses standard library only

### Negative

- **Ephemeral**: Logs lost when process exits (unless captured)
- **No Rotation**: Can't rotate logs (not applicable for short-lived process)
- **No Structure**: Plain text, not JSON (harder to parse)
- **No Levels**: Must use prefixes for severity

### Mitigation

For V2, if structured logging is needed:
- Use `log/slog` (Go 1.21+)
- Write JSON logs to stderr
- Parse logs with `jq` or log aggregation tool

## Testing

### Log Testing

Unit tests can capture stderr:
```go
func TestStartupLogs(t *testing.T) {
    // Capture stderr
    oldStderr := os.Stderr
    r, w, _ := os.Pipe()
    os.Stderr = w

    // Run server
    main()

    // Read logs
    w.Close()
    logs, _ := io.ReadAll(r)
    os.Stderr = oldStderr

    // Assert
    assert.Contains(t, string(logs), "Starting AGM MCP Server")
}
```

### Integration Testing

Test Claude Code integration:
1. Launch server via Claude Code
2. Open Developer Tools
3. Verify logs appear in console
4. Trigger error (invalid config)
5. Verify error logged to stderr

## Monitoring (V2)

For production monitoring, consider:

### Structured Logging

```go
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
logger.Info("startup",
    "version", Version,
    "sessions_dir", cfg.SessionsDir,
    "tools", len(tools))
```

Output:
```json
{"time":"2025-01-15T10:30:45Z","level":"INFO","msg":"startup","version":"1.0.0","sessions_dir":"~/.config/agm/sessions","tools":3}
```

### Log Aggregation

If running multiple MCP server instances:
- Capture stderr to file (one per instance)
- Aggregate with `tail -f /tmp/agm-*.log`
- Parse JSON logs with `jq`:
  ```bash
  tail -f /tmp/agm-*.log | jq 'select(.level == "ERROR")'
  ```

### Metrics Export (V3)

For advanced monitoring:
- Add Prometheus metrics endpoint (HTTP sidecar)
- Export to StatsD/Datadog
- Requires network transport (not stdio)

## Security Considerations

### No Sensitive Data in Logs

Never log:
- Session names (may contain sensitive info)
- Session IDs (unless error context)
- File paths (may reveal directory structure)
- User input (may contain credentials)

### Log Example (Bad)

```go
// BAD: Logs session name
log.Printf("Searching sessions for: %s", query)
```

### Log Example (Good)

```go
// GOOD: No user data
log.Printf("Search completed (%d matches)", len(matches))
```

## References

- Unix stderr convention: https://en.wikipedia.org/wiki/Standard_streams#Standard_error_(stderr)
- MCP Stdio Transport: https://modelcontextprotocol.io/docs/transports/stdio
- Go log package: https://pkg.go.dev/log
- Go slog package: https://pkg.go.dev/log/slog

## Decision Date

2025-01-15

## Reviewers

- vbonnet (author)
