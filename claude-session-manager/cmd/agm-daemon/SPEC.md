# AGM Daemon - Specification

## Overview

The AGM Daemon is a background service that monitors active AGM (AI Guided Manager) sessions running in tmux, detects their state (ready, thinking, blocked, stuck), and exposes session states via HTTP API and file-based status updates. It enables external tools and scripts to query Claude Code session states without directly interacting with tmux.

## Objectives

1. **Real-Time Monitoring**: Continuously poll tmux sessions to detect Claude Code state changes
2. **Multi-Interface Access**: Provide both HTTP API and file-based status for flexible integration
3. **Reliability**: Graceful handling of tmux failures, session cleanup, and daemon lifecycle
4. **Performance**: Sub-second state detection with minimal tmux overhead

## Use Cases

### Primary Use Cases

1. **Session State Monitoring**
   - External scripts query session state via HTTP API
   - Tools check if Claude is ready/thinking/blocked before sending commands
   - Monitoring dashboards display real-time session states

2. **File-Based Status Integration**
   - Shell scripts read status JSON files from `~/.agm/status/`
   - Integration with terminal multiplexers (tmux status bar)
   - Build systems check session readiness before automation

3. **Multi-Session Oversight**
   - List all active sessions with current states
   - Identify stuck or blocked sessions requiring attention
   - Monitor session activity across multiple Claude instances

### Secondary Use Cases

1. **Health Monitoring**
   - Check daemon health via `/health` endpoint
   - Verify daemon is running and monitoring sessions
   - Integration with system monitoring tools

2. **Graceful Shutdown**
   - Handle SIGTERM/SIGINT for clean daemon shutdown
   - Stop monitoring loops and HTTP server gracefully
   - Clean up stale status files on shutdown

## API Specification

### HTTP Endpoints

#### GET /status

**Purpose**: Return status for all monitored sessions

**Response Schema**:
```json
{
  "sessions": [
    {
      "session_name": "session-1",
      "state": "ready",
      "timestamp": "2026-02-11T10:30:00Z",
      "evidence": "Claude prompt (❯) detected",
      "confidence": "high",
      "last_updated": "2026-02-11T10:30:01Z"
    }
  ],
  "count": 1,
  "timestamp": "2026-02-11T10:30:01Z"
}
```

**Status Codes**:
- `200 OK`: Success
- `405 Method Not Allowed`: Non-GET request

#### GET /status/{session-name}

**Purpose**: Return status for specific session

**Response Schema**:
```json
{
  "session_name": "session-1",
  "state": "thinking",
  "timestamp": "2026-02-11T10:30:00Z",
  "evidence": "⣾",
  "confidence": "high",
  "last_updated": "2026-02-11T10:30:01Z"
}
```

**Error Response (Session Not Found)**:
```json
{
  "session_name": "session-1",
  "state": "unknown",
  "timestamp": "2026-02-11T10:30:01Z",
  "error": "Session not found or not being monitored",
  "confidence": "low",
  "last_updated": "2026-02-11T10:30:01Z"
}
```

**Status Codes**:
- `200 OK`: Success (active session)
- `404 Not Found`: Session not being monitored
- `405 Method Not Allowed`: Non-GET request

#### GET /health

**Purpose**: Return daemon health status

**Response Schema**:
```json
{
  "status": "ok",
  "timestamp": "2026-02-11T10:30:01Z",
  "sessions": 3
}
```

**Status Codes**:
- `200 OK`: Daemon is healthy
- `405 Method Not Allowed`: Non-GET request

## State Detection Model

### Session States

| State | Description | Detection Pattern |
|-------|-------------|-------------------|
| `ready` | Claude is idle, waiting for input | Claude prompt `❯` at end of output |
| `thinking` | Claude is processing (spinner visible) | Spinner characters `⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷` |
| `blocked_auth` | y/N authentication prompt | `y/N` or `Y/n` patterns |
| `blocked_input` | AskUserQuestion prompt | Numbered options, choice keywords |
| `stuck` | No token output for >60s | Time since last output exceeded |
| `unknown` | Unable to determine state | No recognizable pattern |

### State Priority Order

Detection priority (highest to lowest):
1. `blocked_auth` - Needs immediate user response
2. `blocked_input` - Waiting for user decision
3. `thinking` - Actively processing
4. `stuck` - No progress detected
5. `ready` - Idle and ready
6. `unknown` - Fallback state

### Confidence Levels

- `high`: Strong pattern match (regex match, explicit markers)
- `medium`: Heuristic-based detection (timeout thresholds)
- `low`: Unknown state or no pattern match

## File-Based Status

### Status File Format

**Location**: `~/.agm/status/{session-name}.json`

**Schema**: Same as HTTP response for single session

**Example**:
```json
{
  "session_name": "session-1",
  "state": "ready",
  "timestamp": "2026-02-11T10:30:00Z",
  "evidence": "Claude prompt (❯) detected",
  "confidence": "high",
  "last_updated": "2026-02-11T10:30:01Z"
}
```

### File Operations

- **Write**: Atomic write via temp file + rename
- **Read**: Standard JSON parsing
- **Delete**: Remove status file when session terminates
- **Directory**: Auto-created with `0755` permissions

## Configuration

### Command-Line Flags

```bash
agm-daemon [OPTIONS]

Options:
  -port int
        HTTP API port (default 8765)
  -status-dir string
        Status file directory (default "~/.agm/status")
  -poll-interval duration
        Session polling interval (default 2s)
```

### Default Values

- **Port**: `8765`
- **Status Directory**: `~/.agm/status`
- **Poll Interval**: `2s` (2 seconds)

### Configuration Precedence

1. Command-line flags (highest priority)
2. Default values (lowest priority)

## Monitoring Loop

### Polling Cycle

```
Start daemon
    ↓
Initialize HTTP server
    ↓
Start monitoring loop (every 2s)
    ↓
┌─────────────────────────────┐
│ Poll tmux sessions          │
│   - List all tmux sessions  │
│   - For each session:       │
│     - Capture pane output   │
│     - Detect state          │
│     - Update cache          │
│     - Write status file     │
│   - Clean up stale sessions │
└─────────────────────────────┘
    ↓
Wait 2s
    ↓
Repeat until shutdown signal
    ↓
Graceful shutdown
```

### Session Lifecycle

1. **Discovery**: Tmux session detected via `tmux list-sessions`
2. **Monitor Creation**: New `SessionMonitor` created for session
3. **State Polling**: Pane output captured every 2s, state detected
4. **Cache Update**: In-memory cache and HTTP server updated
5. **File Write**: Status written to `~/.agm/status/{session-name}.json`
6. **Cleanup**: Monitor deleted when session no longer exists

## Performance Requirements

### Response Time Targets

| Endpoint | Target | Rationale |
|----------|--------|-----------|
| GET /status | <50ms | In-memory read, no disk I/O |
| GET /status/{name} | <10ms | Hash map lookup |
| GET /health | <5ms | Simple counter read |

### Polling Overhead

- **Tmux Calls**: 2 calls per session per poll cycle
  - `tmux list-sessions` (once per cycle)
  - `tmux capture-pane` (per session)
- **Poll Interval**: 2s default (configurable)
- **Expected Sessions**: 1-10 (typical), up to 50 (supported)

### Resource Usage

- **Memory**: ~1KB per session (in-memory cache)
- **Disk**: ~500 bytes per status file
- **CPU**: Minimal (state detection is regex-based)

## Error Handling

### Error Categories

1. **Initialization Errors** (Fatal)
   - Failed to create status directory
   - Failed to start HTTP server
   - Action: Log error, exit process

2. **Tmux Errors** (Non-Fatal)
   - No tmux sessions available
   - Tmux server not running
   - Action: Skip polling cycle, continue monitoring

3. **Session Errors** (Non-Fatal)
   - Session disappeared during polling
   - Permission denied on pane capture
   - Action: Skip session, remove from cache

4. **File Write Errors** (Non-Fatal)
   - Failed to write status file
   - Action: Log warning, continue monitoring

### Error Logging

- **Destination**: Stdout (standard Go `log` package)
- **Format**: Timestamp + message
- **Level**: Errors and warnings only (no debug output)

## Shutdown Behavior

### Graceful Shutdown

1. **Signal Handling**: Catch SIGTERM, SIGINT
2. **Stop Monitoring**: Cancel context, stop polling loop
3. **Stop HTTP Server**: Close server, drain connections
4. **Wait for Goroutines**: Synchronize via `sync.WaitGroup`
5. **Exit**: Clean process termination

### Cleanup on Shutdown

- **Status Files**: Preserved (manual cleanup required)
- **In-Memory Cache**: Discarded
- **HTTP Server**: Stopped gracefully

## Security & Privacy

### Security Principles

1. **Local Only**: HTTP server binds to `localhost` (no network exposure)
2. **Read-Only**: Daemon only reads tmux output, never writes commands
3. **No Credentials**: No API keys, passwords, or sensitive data exposed
4. **Session Isolation**: Monitors only accessible tmux sessions

### Privacy Guarantees

- **Exposed Data**: Session name, state, timestamps, evidence snippets
- **Protected Data**: Full conversation history, user prompts, agent responses

## Dependencies

### External Dependencies

1. **Tmux**: Required for session monitoring
   - Commands: `list-sessions`, `capture-pane`
   - Version: 2.6+ (for format string support)

2. **Go Standard Library**: HTTP server, JSON encoding, signal handling

### Internal Dependencies

1. **internal/daemon**: Daemon orchestration
2. **internal/api**: HTTP server, status file writer
3. **internal/state**: State detection logic
4. **internal/tmux**: Tmux interaction utilities

## Deployment

### Build

```bash
cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager
go build -o agm-daemon cmd/agm-daemon/*.go
```

### Installation

```bash
# Copy to user bin
cp agm-daemon ~/bin/

# Make executable
chmod +x ~/bin/agm-daemon
```

### Running

```bash
# Foreground (for testing)
agm-daemon

# Background (production)
agm-daemon > /tmp/agm-daemon.log 2>&1 &

# Custom configuration
agm-daemon -port 9000 -poll-interval 5s
```

### Systemd Service (Optional)

```ini
[Unit]
Description=AGM Daemon - Session State Monitor
After=network.target

[Service]
Type=simple
ExecStart=/home/user/bin/agm-daemon
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

## Future Enhancements (V2+)

1. **WebSocket Support**: Real-time state updates to clients
2. **Metrics Endpoint**: Prometheus-compatible metrics
3. **Authentication**: Optional API key protection
4. **Configuration File**: YAML config support
5. **Advanced State Detection**: ML-based state inference
6. **Session Control**: Start/stop/restart sessions via API
7. **Event Logging**: Persist state change events to disk

## Testing Requirements

### Unit Tests

- State detection pattern matching
- HTTP endpoint handlers
- Status file read/write
- Session cleanup logic
- Graceful shutdown

### Integration Tests

- End-to-end tmux monitoring
- HTTP API compliance
- File-based status updates
- Multi-session handling
- Error recovery

### Performance Tests

- Response time benchmarks
- Polling overhead measurement
- Memory usage tracking
- Concurrent request handling

## References

- AGM Session Manager: ~/src/ws/oss/repos/ai-tools/main/claude-session-manager/
- Tmux Documentation: https://man.openbsd.org/tmux
- Go HTTP Server: https://pkg.go.dev/net/http
- State Detection: internal/state/detector.go
