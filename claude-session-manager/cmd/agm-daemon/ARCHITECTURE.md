# AGM Daemon - Architecture

## System Overview

The AGM Daemon is a background service that continuously monitors tmux sessions running Claude Code, detects session states through visual parsing of terminal output, and exposes state information via both HTTP API and file-based status updates. It runs as a standalone process with minimal overhead, providing real-time session state visibility to external tools.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    External Clients                              │
│                                                                  │
│  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────┐ │
│  │ Shell Scripts    │  │ Monitoring Tools │  │ tmux Status   │ │
│  │ (curl HTTP API)  │  │ (GET /status)    │  │ (read files)  │ │
│  └──────────┬───────┘  └────────┬─────────┘  └───────┬───────┘ │
└─────────────┼────────────────────┼────────────────────┼─────────┘
              │                    │                    │
              │ HTTP GET           │ HTTP GET           │ File Read
              ▼                    ▼                    ▼
┌─────────────────────────────────────────────────────────────────┐
│                        AGM Daemon Process                        │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ main.go - Entry Point                                     │  │
│  │ - Flag parsing                                            │  │
│  │ - Daemon creation                                         │  │
│  │ - Lifecycle management                                    │  │
│  └──────────────────┬───────────────────────────────────────┘  │
│                     │                                           │
│  ┌──────────────────▼───────────────────────────────────────┐  │
│  │ internal/daemon - Daemon Orchestration                    │  │
│  │                                                            │  │
│  │  ┌────────────────┐  ┌─────────────────┐  ┌────────────┐ │  │
│  │  │ HTTP Server    │  │ Monitoring Loop │  │ Session    │ │  │
│  │  │ (goroutine)    │  │ (goroutine)     │  │ Cache      │ │  │
│  │  │                │  │                 │  │ (sync.Map) │ │  │
│  │  │ - GET /status  │  │ - Poll tmux     │  │            │ │  │
│  │  │ - GET /health  │  │ - Detect state  │  │ - Thread   │ │  │
│  │  │                │  │ - Update cache  │  │   safe     │ │  │
│  │  │                │  │ - Write files   │  │ - In-mem   │ │  │
│  │  └────────┬───────┘  └────────┬────────┘  └─────┬──────┘ │  │
│  └───────────┼──────────────────┼──────────────────┼────────┘  │
│              │                   │                  │           │
│  ┌───────────▼───────┐  ┌───────▼────────┐  ┌──────▼───────┐  │
│  │ internal/api      │  │ internal/state │  │ internal/api │  │
│  │ - Server          │  │ - Detector     │  │ - StatusFile │  │
│  │ - StatusResponse  │  │ - Patterns     │  │   Writer     │  │
│  │ - Endpoints       │  │ - Confidence   │  │ - Atomic     │  │
│  └───────────────────┘  └────────┬───────┘  └──────┬───────┘  │
│                                  │                  │           │
└──────────────────────────────────┼──────────────────┼───────────┘
                                   │                  │
                                   ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Internal Libraries                          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ internal/tmux                                             │  │
│  │ - ListSessions()                                          │  │
│  │ - CapturePaneOutput()                                     │  │
│  └──────────────────┬───────────────────────────────────────┘  │
└─────────────────────┼───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Tmux Session Layer                            │
│                                                                  │
│  Active tmux sessions:                                          │
│  ├── session-1 (Claude Code running)                           │
│  ├── session-2 (Claude Code ready)                             │
│  └── session-3 (Claude Code thinking)                          │
│                                                                  │
│  Status files:                                                  │
│  ~/.agm/status/                                                 │
│  ├── session-1.json ← Written by daemon                        │
│  ├── session-2.json                                             │
│  └── session-3.json                                             │
└─────────────────────────────────────────────────────────────────┘
```

## Component Architecture

### 1. main.go - Entry Point

**Responsibilities**:
- Parse command-line flags
- Create daemon instance
- Start daemon lifecycle
- Block until shutdown signal

**Key Functions**:
```go
func main()
    → flag.Parse()
    → daemon.NewDaemon(port, statusDir, pollInterval)
    → daemon.Start()
```

**Flag Handling**:
- `-port`: HTTP API port (default 8765)
- `-status-dir`: Status file directory (default `~/.agm/status`)
- `-poll-interval`: Session polling interval (default 2s)

**Lifecycle**:
```
Parse flags
    ↓
Create daemon
    ↓
Start daemon (blocking)
    ↓
[Runs until SIGTERM/SIGINT]
    ↓
Graceful shutdown
```

### 2. internal/daemon - Daemon Orchestration

**Responsibilities**:
- Coordinate HTTP server and monitoring loop
- Manage session cache
- Handle shutdown signals
- Synchronize goroutines

**Daemon Struct**:
```go
type Daemon struct {
    port          int
    statusDir     string
    pollInterval  time.Duration
    server        *api.Server
    statusWriter  *api.StatusFileWriter
    detector      *state.Detector
    sessions      map[string]*SessionMonitor
    mu            sync.RWMutex
    ctx           context.Context
    cancel        context.CancelFunc
    wg            sync.WaitGroup
}
```

**Session Monitor**:
```go
type SessionMonitor struct {
    Name         string
    LastOutput   time.Time
    LastState    state.State
    PaneID       string
}
```

**Key Functions**:
```go
func NewDaemon(port, statusDir, pollInterval) → *Daemon
func (d *Daemon) Start() → error
func (d *Daemon) Stop() → error
func (d *Daemon) monitoringSessions()
func (d *Daemon) pollSessions()
func (d *Daemon) monitorSession(sessionName)
func (d *Daemon) cleanupStaleSessions(activeSessions)
```

**Goroutine Model**:
- **Main Goroutine**: Waits for shutdown signal
- **HTTP Server Goroutine**: Serves HTTP requests
- **Monitoring Goroutine**: Polls tmux sessions

**Synchronization**:
- `sync.RWMutex` protects session cache
- `sync.WaitGroup` tracks active goroutines
- `context.Context` signals shutdown to goroutines

### 3. internal/api - HTTP Server

**Responsibilities**:
- Serve HTTP API endpoints
- Manage in-memory session cache
- Thread-safe cache updates
- JSON response formatting

**Server Struct**:
```go
type Server struct {
    port      int
    detector  *state.Detector
    sessions  map[string]*StatusResponse
    mu        sync.RWMutex
    server    *http.Server
}
```

**Endpoints**:
```go
GET /status        → handleStatus()        // All sessions
GET /status/{name} → handleSessionStatus() // Single session
GET /health        → handleHealth()        // Server health
```

**Key Functions**:
```go
func NewServer(port, detector) → *Server
func (s *Server) Start() → error
func (s *Server) Stop() → error
func (s *Server) UpdateSessionState(name, result)
```

**Cache Update Flow**:
```
Monitoring loop detects state
    ↓
UpdateSessionState(name, result)
    ↓
Acquire write lock
    ↓
Update sessions map
    ↓
Release lock
    ↓
HTTP requests read from cache (read lock)
```

### 4. internal/api - Status File Writer

**Responsibilities**:
- Write status JSON files atomically
- Read status files on demand
- Delete status files for terminated sessions
- Manage status directory lifecycle

**StatusFileWriter Struct**:
```go
type StatusFileWriter struct {
    baseDir string
}
```

**Key Functions**:
```go
func NewStatusFileWriter(baseDir) → *StatusFileWriter
func (w *StatusFileWriter) WriteStatus(name, result) → error
func (w *StatusFileWriter) ReadStatus(name) → *StatusResponse
func (w *StatusFileWriter) DeleteStatus(name) → error
func (w *StatusFileWriter) ListSessions() → []string
```

**Atomic Write Strategy**:
```
1. Marshal JSON
2. Write to temp file (session-name.json.tmp)
3. Rename temp file to final path
4. Delete temp file on error
```

**File Structure**:
```
~/.agm/status/
├── session-1.json
├── session-2.json
└── session-3.json
```

### 5. internal/state - State Detector

**Responsibilities**:
- Analyze tmux pane output
- Match state detection patterns
- Calculate confidence scores
- Extract evidence snippets

**Detector Struct**:
```go
type Detector struct {
    thinkingPattern     *regexp.Regexp
    blockedAuthPattern  *regexp.Regexp
    blockedInputPattern *regexp.Regexp
    readyPattern        *regexp.Regexp
    stuckThreshold      time.Duration
}
```

**Detection Result**:
```go
type DetectionResult struct {
    State      State
    Timestamp  time.Time
    Evidence   string
    Confidence string
}
```

**Key Functions**:
```go
func NewDetector() → *Detector
func (d *Detector) DetectState(output, lastOutputTime) → DetectionResult
func (d *Detector) extractEvidence(output, pattern, contextChars) → string
```

**Detection Patterns**:
| State | Regex Pattern |
|-------|---------------|
| Thinking | `[⣾⣽⣻⢿⡿⣟⣯⣷]` |
| Blocked Auth | `(?i)\b([yY]/[nN]|[nN]/[yY])\b` |
| Blocked Input | Numbered options, choice keywords |
| Ready | `❯\s*$` |

**Detection Priority**:
1. Blocked Auth (highest)
2. Blocked Input
3. Thinking
4. Stuck (timeout-based)
5. Ready
6. Unknown (fallback)

### 6. internal/tmux - Tmux Utilities

**Responsibilities**:
- Execute tmux commands
- Parse tmux output
- Handle tmux errors
- Timeout protection

**Key Functions Used by Daemon**:
```go
func ListSessions() → []string
func CapturePaneOutput(sessionName, lines) → string
```

**Tmux Commands**:
```bash
# List sessions
tmux -S ~/.agm/tmux.sock list-sessions -F "#{session_name}"

# Capture pane output (last 50 lines)
tmux -S ~/.agm/tmux.sock capture-pane -t session-1 -p -S -50
```

## Data Flow

### Monitoring Loop Flow

```
Timer triggers (every 2s)
    ↓
List all tmux sessions
    ↓
For each session:
    ↓
    Capture pane output (50 lines)
        ↓
    Detect state (visual parsing)
        ↓
    Get/create SessionMonitor
        ↓
    Update LastState, LastOutput
        ↓
    Update HTTP server cache
        ↓
    Write status file
    ↓
Clean up stale sessions
    ↓
Wait 2s
    ↓
Repeat
```

### HTTP Request Flow

```
Client sends GET /status/session-1
    ↓
HTTP handler receives request
    ↓
Extract session name from path
    ↓
Acquire read lock
    ↓
Lookup session in cache
    ↓
Release read lock
    ↓
Found? → Return JSON response (200)
Not found? → Return error response (404)
```

### State Detection Flow

```
Capture pane output (raw text)
    ↓
Extract last 50 lines
    ↓
Run detection patterns (priority order):
    1. Check blocked_auth pattern
    2. Check blocked_input pattern
    3. Check thinking pattern
    4. Check timeout (stuck detection)
    5. Check ready pattern
    6. Fallback to unknown
    ↓
Extract evidence snippet (context around match)
    ↓
Return DetectionResult
    ↓
Update cache and write file
```

## Concurrency Model

### Thread Safety

1. **Session Cache**: Protected by `sync.RWMutex`
   - Concurrent reads allowed (GET requests)
   - Exclusive writes (monitoring loop updates)

2. **HTTP Server Cache**: Protected by `sync.RWMutex`
   - Concurrent reads (multiple HTTP requests)
   - Exclusive writes (UpdateSessionState)

3. **Goroutine Synchronization**: `sync.WaitGroup`
   - Tracks HTTP server and monitoring loop
   - Ensures clean shutdown (wait for goroutines)

### Goroutine Lifecycle

```
main() starts
    ↓
Start HTTP server goroutine
    ↓
Start monitoring loop goroutine
    ↓
Wait for shutdown signal
    ↓
Cancel context (signal goroutines)
    ↓
Wait for goroutines (WaitGroup)
    ↓
Exit process
```

### Lock Ordering

To prevent deadlocks:
1. Daemon.mu → API.Server.mu (never reversed)
2. Short critical sections (no I/O under lock)
3. Read locks released quickly

## Error Handling Strategy

### Error Categories

1. **Initialization Errors** (Fatal)
   - Failed to create status directory
   - Failed to start HTTP server
   - Action: Log error, exit process

2. **Polling Errors** (Non-Fatal)
   - No tmux sessions available
   - Tmux server not running
   - Action: Skip cycle, log warning, continue

3. **Session Errors** (Non-Fatal)
   - Session disappeared during poll
   - Permission denied on capture
   - Action: Skip session, log warning, continue

4. **File Write Errors** (Non-Fatal)
   - Failed to write status file
   - Action: Log warning, continue monitoring

### Error Recovery

```
Error occurs
    ↓
Log error to stdout
    ↓
Non-fatal? → Continue operation
Fatal? → Exit process with error code
```

## Performance Architecture

### Optimization Strategies

1. **In-Memory Cache**: Avoid disk I/O for HTTP requests
2. **Minimal Tmux Calls**: Batch session list, capture per session
3. **Efficient Regex**: Compiled patterns, single-pass matching
4. **Thread-Safe Maps**: RWMutex allows concurrent reads

### Performance Bottlenecks

| Component | Bottleneck | Mitigation |
|-----------|-----------|------------|
| Tmux Calls | Process spawning overhead | Batch calls, long poll interval |
| State Detection | Regex matching | Compiled patterns, priority ordering |
| File Writes | Disk I/O | Atomic writes, async operation |
| HTTP Requests | Lock contention | Read locks, short critical sections |

### Scalability Limits

- **Sessions**: Designed for 1-50 sessions (typical: 1-10)
- **Poll Interval**: 2s default (adjustable for load)
- **HTTP Throughput**: 1000+ req/s (in-memory reads)
- **Memory**: ~1KB per session (~50KB for 50 sessions)

## Security Architecture

### Security Principles

1. **Local Only**: HTTP server binds to `127.0.0.1:8765`
2. **Read-Only**: Daemon only reads tmux output
3. **No Authentication**: Local process, trusted environment
4. **Minimal Privileges**: Runs as user, no root required

### Trust Boundaries

```
┌─────────────────────────────────────┐
│  External Clients (Same User)       │
│  - Shell scripts                    │
│  - Monitoring tools                 │
│  - tmux status bar                  │
└──────────────┬──────────────────────┘
               │ HTTP (localhost)
               │ File read (user perms)
               ▼
┌─────────────────────────────────────┐
│  AGM Daemon (Trusted)               │
│  - Runs as user                     │
│  - Local process                    │
└──────────────┬──────────────────────┘
               │ Tmux commands
               ▼
┌─────────────────────────────────────┐
│  Tmux Sessions (Protected)          │
│  - User-owned sessions              │
│  - Read-only access                 │
└─────────────────────────────────────┘
```

### Access Control

- **File Permissions**: Status files `0644` (user read/write, group/other read)
- **Directory Permissions**: Status directory `0755` (user full, group/other read+execute)
- **Network Binding**: `127.0.0.1` only (no external access)

## Deployment Architecture

### Process Management

```bash
# Manual start (foreground)
agm-daemon

# Background with logging
agm-daemon > /tmp/agm-daemon.log 2>&1 &

# Systemd service
systemctl --user start agm-daemon
```

### Lifecycle Management

1. **Start**: Parse flags, create daemon, start goroutines
2. **Run**: HTTP server + monitoring loop run indefinitely
3. **Shutdown**: SIGTERM/SIGINT triggers graceful shutdown
4. **Cleanup**: Stop goroutines, close server, exit

### Health Monitoring

```bash
# Check daemon health
curl http://localhost:8765/health

# Check session count
curl http://localhost:8765/status | jq '.count'

# Monitor daemon logs
tail -f /tmp/agm-daemon.log
```

## Monitoring & Observability

### Logging

**Log Destination**: Stdout (redirect to file as needed)

**Log Format**: Go standard `log` package
```
2026/02/11 10:30:00 AGM Daemon starting...
2026/02/11 10:30:00   Port: 8765
2026/02/11 10:30:00   Status dir: ~/.agm/status
2026/02/11 10:30:00   Poll interval: 2s
```

**Log Events**:
- Daemon startup
- Configuration values
- Session discovery
- State changes (optional)
- Errors and warnings
- Shutdown signal

### Metrics (Future)

- Sessions monitored
- States detected (by type)
- HTTP requests (by endpoint)
- Polling cycle duration
- Error rate

## Testing Architecture

### Unit Test Structure

```
internal/daemon/
├── daemon_test.go        # Daemon lifecycle tests
internal/api/
├── server_test.go        # HTTP endpoint tests
├── status_file_test.go   # File I/O tests
internal/state/
├── detector_test.go      # State detection tests
```

### Test Categories

1. **Daemon Tests**
   - Initialization
   - Goroutine lifecycle
   - Shutdown behavior
   - Session cleanup

2. **API Tests**
   - HTTP endpoint responses
   - Error handling
   - Concurrent request handling
   - Cache update synchronization

3. **State Detection Tests**
   - Pattern matching accuracy
   - Priority ordering
   - Confidence scoring
   - Evidence extraction

4. **Integration Tests**
   - End-to-end tmux monitoring
   - File-based status updates
   - Multi-session handling
   - Error recovery

## Dependencies

### External Dependencies

1. **Tmux**: Session monitoring
   - Version: 2.6+ (format string support)
   - Commands: `list-sessions`, `capture-pane`

2. **Go Standard Library**:
   - `net/http`: HTTP server
   - `encoding/json`: JSON marshaling
   - `os/signal`: Signal handling
   - `sync`: Concurrency primitives

### Internal Dependencies

```
cmd/agm-daemon/main.go
└── internal/daemon
    ├── internal/api
    │   └── internal/state
    └── internal/tmux
```

### Dependency Graph

```
agm-daemon
├── internal/daemon
│   ├── internal/api (server + status files)
│   ├── internal/state (detector)
│   └── internal/tmux (session commands)
└── standard library (http, json, sync, signal)
```

## Future Architecture Enhancements

### V2 Features

1. **WebSocket Support**
   - Real-time state updates to clients
   - Server-sent events for state changes
   - Lower latency than polling

2. **Metrics Endpoint**
   - Prometheus-compatible `/metrics`
   - Session state histograms
   - Polling cycle duration

3. **Configuration File**
   - YAML config support
   - Per-session polling intervals
   - Custom state patterns

### V3 Features

1. **Session Control API**
   - POST /sessions/{name}/restart
   - POST /sessions/{name}/stop
   - Session lifecycle management

2. **Event Logging**
   - Persist state change events
   - Queryable event history
   - Audit trail for debugging

3. **Advanced State Detection**
   - ML-based state inference
   - Custom pattern definitions
   - Confidence calibration

## Design Patterns

### Patterns Used

1. **Observer Pattern**: Daemon monitors tmux sessions
2. **Producer-Consumer**: Monitoring loop produces, HTTP serves consumes
3. **Singleton Cache**: Single in-memory session cache
4. **Atomic File Operations**: Temp file + rename pattern

### Anti-Patterns Avoided

1. **No Busy Loops**: Timer-based polling with configurable interval
2. **No Global State**: Config passed via struct fields
3. **No Blocking I/O Under Lock**: File writes outside critical sections
4. **No Premature Optimization**: Simple map-based cache

## Code Organization Principles

1. **Single Responsibility**: Each package has one clear purpose
2. **Minimal Coupling**: Daemon orchestrates, packages implement
3. **Clear Interfaces**: HTTP API, file API, tmux API
4. **Error Transparency**: Errors logged, context preserved
5. **Test-Friendly**: Functions accept interfaces, avoid globals
