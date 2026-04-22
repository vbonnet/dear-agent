# OpenCode SSE Adapter - Architecture

**Package**: `internal/monitor/opencode`
**Version**: 1.0
**Status**: Design
**Created**: 2026-03-06
**Bead**: oss-lc87

---

## Overview

The OpenCode SSE Adapter provides real-time session state monitoring for OpenCode agent sessions by subscribing to OpenCode's native Server-Sent Events (SSE) stream. This eliminates the need for tmux screen scraping and provides millisecond-level state detection latency.

### Goals

1. **Real-time state detection** - Subscribe to OpenCode's `/event` SSE endpoint
2. **EventBus integration** - Publish state changes to AGM's canonical EventBus
3. **Connection resilience** - Auto-reconnect with exponential backoff
4. **Zero screen scraping** - Native event stream eliminates tmux dependency
5. **Production reliability** - Health monitoring, error handling, graceful degradation

### Success Criteria

- SSE connection establishes within 100ms of adapter start
- State changes detected within 50-100ms of occurrence
- Auto-reconnect recovers from network failures within 30s
- Malformed events logged but don't crash adapter
- EventBus receives well-formed SessionStateChangeEvent for all state transitions

---

## Component Architecture

### Component Diagram

```
┌────────────────────────────────────────────────────────────┐
│              OpenCode SSE Adapter                          │
│                                                            │
│  ┌──────────────┐    ┌──────────────┐    ┌─────────────┐ │
│  │  SSE Client  │───▶│Event Parser  │───▶│  Publisher  │ │
│  │              │    │              │    │             │ │
│  │ - Connect    │    │ - Validate   │    │ - EventBus  │ │
│  │ - Reconnect  │    │ - Map states │    │ - Metadata  │ │
│  │ - Health     │    │ - Extract    │    │ - Timestamp │ │
│  └──────┬───────┘    └──────────────┘    └──────┬──────┘ │
│         │                                         │        │
│         │            ┌──────────────┐             │        │
│         └───────────▶│  Lifecycle   │◀────────────┘        │
│                      │   Manager    │                      │
│                      │              │                      │
│                      │ - Start/Stop │                      │
│                      │ - Health     │                      │
│                      │ - Session ID │                      │
│                      └──────────────┘                      │
└────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │   AGM EventBus   │
                    │  (Integration    │
                    │      Layer)      │
                    └──────────────────┘
```

### Data Flow

```
OpenCode Server
    │
    │ (SSE /event stream)
    │
    ├─► permission.asked
    ├─► tool.execute.before
    ├─► tool.execute.after
    ├─► session.created
    └─► session.closed
         │
         ▼
    SSE Client (sse_adapter.go)
         │
         ├─► Validate HTTP 200, text/event-stream
         ├─► Parse SSE message format
         └─► Extract event JSON from data: field
              │
              ▼
    Event Parser (event_parser.go)
         │
         ├─► Unmarshal JSON
         ├─► Validate event schema
         ├─► Map OpenCode type → AGM state
         └─► Extract metadata (permission IDs, tool names)
              │
              ▼
    EventBus Publisher (publisher.go)
         │
         ├─► Create SessionStateChangeEvent
         ├─► Add source: "opencode-sse"
         ├─► Add agent: "opencode"
         └─► Publish to EventBus
              │
              ▼
    AGM EventBus (internal/eventbus)
         │
         ├─► State File Writer (daemon)
         ├─► Notification Manager (v4)
         └─► Tmux Status (consumer)
```

---

## Components

### 1. SSE Client (`sse_adapter.go`)

**Purpose**: Manage HTTP connection to OpenCode's SSE endpoint with auto-reconnect.

**Interface**:
```go
type SSEAdapter struct {
    serverURL   string
    client      *http.Client
    eventBus    *eventbus.EventBus
    sessionID   string
    reconnect   ReconnectConfig
    ctx         context.Context
    cancel      context.CancelFunc
    connected   atomic.Bool
    lastEvent   atomic.Value // time.Time
}

func NewAdapter(eventBus *eventbus.EventBus, config Config) *SSEAdapter
func (a *SSEAdapter) Start(ctx context.Context) error
func (a *SSEAdapter) Stop(ctx context.Context) error
func (a *SSEAdapter) Health() HealthStatus
func (a *SSEAdapter) Name() string
```

**Connection Management**:

```go
func NewSSEAdapter(config Config) *SSEAdapter {
    return &SSEAdapter{
        serverURL: config.ServerURL,
        client: &http.Client{
            Timeout: 0, // No timeout for streaming connection
            Transport: &http.Transport{
                DialContext: (&net.Dialer{
                    Timeout:   10 * time.Second,  // Connection timeout
                    KeepAlive: 30 * time.Second,
                }).DialContext,
                TLSHandshakeTimeout:   10 * time.Second,
                ResponseHeaderTimeout: 10 * time.Second,  // Header must arrive within 10s
                IdleConnTimeout:       90 * time.Second,
            },
        },
        // ... other fields
    }
}

func (a *SSEAdapter) connect() error {
    req, _ := http.NewRequestWithContext(a.ctx, "GET", a.serverURL+"/event", nil)
    req.Header.Set("Accept", "text/event-stream")
    req.Header.Set("Cache-Control", "no-cache")

    resp, err := a.client.Do(req)
    if err != nil {
        return fmt.Errorf("connection failed: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
        return fmt.Errorf("invalid content-type: %s", resp.Header.Get("Content-Type"))
    }

    a.connected.Store(true)
    go a.readEvents(resp.Body)
    return nil
}
```

**Event Reading Loop**:

```go
func (a *SSEAdapter) readEvents(body io.ReadCloser) {
    defer body.Close()
    scanner := bufio.NewScanner(body)

    for scanner.Scan() {
        line := scanner.Text()

        // SSE format: "data: {json}"
        if strings.HasPrefix(line, "data: ") {
            eventData := strings.TrimPrefix(line, "data: ")
            a.handleEvent([]byte(eventData))
        }
    }

    // Connection closed
    a.connected.Store(false)
    if a.ctx.Err() == nil {
        // Context not cancelled, reconnect
        a.scheduleReconnect()
    }
}
```

**Auto-Reconnect with Exponential Backoff**:

```go
type ReconnectConfig struct {
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   int
}

func (a *SSEAdapter) scheduleReconnect() {
    delay := a.reconnect.InitialDelay
    attempts := 0

    for {
        select {
        case <-a.ctx.Done():
            return // Shutdown
        case <-time.After(delay):
            log.Info("Reconnecting to OpenCode SSE (attempt %d)...", attempts+1)

            if err := a.connect(); err != nil {
                log.Warn("Reconnect failed: %v", err)
                attempts++

                // Exponential backoff
                delay = delay * time.Duration(a.reconnect.Multiplier)
                if delay > a.reconnect.MaxDelay {
                    delay = a.reconnect.MaxDelay
                }
                continue
            }

            log.Info("Reconnected successfully")
            return
        }
    }
}
```

**Health Check**:

```go
func (a *SSEAdapter) Health() HealthStatus {
    connected := a.connected.Load()
    lastEvent := a.lastEvent.Load().(time.Time)
    lastHeartbeat := a.lastHeartbeat.Load().(time.Time)

    status := HealthStatus{
        Connected: connected,
        LastEvent: lastEvent,
        Metadata: map[string]interface{}{
            "server_url": a.serverURL,
        },
    }

    if !connected {
        status.Error = fmt.Errorf("SSE connection down")
        return status
    }

    // Use heartbeat, not event timestamp (prevents false positives for idle sessions)
    if time.Since(lastHeartbeat) > 5*time.Minute {
        status.Error = fmt.Errorf("no heartbeat for 5 minutes (connection may be dead)")
    }

    return status
}
```

### 2. Event Parser (`event_parser.go`)

**Purpose**: Transform OpenCode SSE events into AGM state transitions.

**Event Schema (OpenCode)**:

```json
{
  "type": "permission.asked",
  "timestamp": 1709654321,
  "properties": {
    "permission": {
      "id": "perm-123",
      "action": "file.write",
      "path": "~/main.go"
    }
  }
}
```

**State Mapping**:

```go
type EventParser struct{}

func (p *EventParser) Parse(data []byte) (*AGMEvent, error) {
    var rawEvent OpenCodeEvent
    if err := json.Unmarshal(data, &rawEvent); err != nil {
        return nil, fmt.Errorf("invalid JSON: %w", err)
    }

    if err := p.validate(rawEvent); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }

    agmState := p.mapState(rawEvent.Type)
    metadata := p.extractMetadata(rawEvent)

    return &AGMEvent{
        State:     agmState,
        Timestamp: rawEvent.Timestamp,
        Metadata:  metadata,
    }, nil
}

func (p *EventParser) mapState(eventType string) string {
    switch eventType {
    case "permission.asked":
        return "AWAITING_PERMISSION"
    case "tool.execute.before":
        return "THINKING"
    case "tool.execute.after":
        return "IDLE"
    case "session.created":
        return "READY"
    case "session.closed":
        return "TERMINATED"
    default:
        log.Warn("Unknown event type: %s, defaulting to THINKING", eventType)
        return "THINKING"
    }
}

func (p *EventParser) extractMetadata(event OpenCodeEvent) map[string]interface{} {
    meta := map[string]interface{}{
        "event_type": event.Type,
    }

    // Add event-specific metadata
    switch event.Type {
    case "permission.asked":
        if perm, ok := event.Properties["permission"].(map[string]interface{}); ok {
            meta["permission_id"] = perm["id"]
            meta["action"] = perm["action"]
            meta["path"] = perm["path"]
        }
    case "tool.execute.before", "tool.execute.after":
        if tool, ok := event.Properties["tool"].(string); ok {
            meta["tool_name"] = tool
        }
    }

    return meta
}
```

**Validation**:

```go
func (p *EventParser) validate(event OpenCodeEvent) error {
    if event.Type == "" {
        return fmt.Errorf("missing event type")
    }

    if event.Timestamp == 0 {
        return fmt.Errorf("missing timestamp")
    }

    return nil
}
```

### 3. EventBus Publisher (`publisher.go`)

**Purpose**: Publish parsed events to AGM's EventBus.

**Implementation**:

```go
type Publisher struct {
    eventBus  *eventbus.EventBus
    sessionID string
}

func (p *Publisher) Publish(agmEvent *AGMEvent) error {
    event := eventbus.SessionStateChangeEvent{
        SessionID: p.sessionID,
        State:     agmEvent.State,
        Timestamp: agmEvent.Timestamp,
        Source:    "opencode-sse",
        Agent:     "opencode",
        Metadata:  agmEvent.Metadata,
    }

    return p.eventBus.Publish(eventbus.EventSessionStateChange, event)
}
```

**Error Handling**:

```go
func (p *Publisher) PublishWithRetry(agmEvent *AGMEvent) error {
    maxRetries := 3
    backoff := 100 * time.Millisecond

    for i := 0; i < maxRetries; i++ {
        err := p.Publish(agmEvent)
        if err == nil {
            return nil
        }

        log.Warn("EventBus publish failed (attempt %d/%d): %v", i+1, maxRetries, err)
        time.Sleep(backoff)
        backoff *= 2
    }

    return fmt.Errorf("failed to publish after %d retries", maxRetries)
}
```

### 4. Lifecycle Manager (`lifecycle.go`)

**Purpose**: Coordinate adapter startup, shutdown, health checks, and session ID mapping.

**Adapter Interface Implementation**:

```go
type Adapter struct {
    sseClient *SSEAdapter
    parser    *EventParser
    publisher *Publisher
    config    Config
}

func NewAdapter(eventBus *eventbus.EventBus, config Config) *Adapter {
    sseClient := NewSSEAdapter(eventBus, config)
    parser := NewEventParser()
    publisher := NewPublisher(eventBus, config.SessionID)

    return &Adapter{
        sseClient: sseClient,
        parser:    parser,
        publisher: publisher,
        config:    config,
    }
}

func (a *Adapter) Start(ctx context.Context) error {
    // Health probe to server
    if err := a.healthProbe(); err != nil {
        return fmt.Errorf("server health check failed: %w", err)
    }

    // Start SSE client
    return a.sseClient.Start(ctx)
}

func (a *Adapter) Stop(ctx context.Context) error {
    return a.sseClient.Stop(ctx)
}

func (a *Adapter) Health() HealthStatus {
    return a.sseClient.Health()
}

func (a *Adapter) Name() string {
    return "opencode-sse"
}
```

**Server Discovery**:

```go
func (a *Adapter) healthProbe() error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, "GET", a.config.ServerURL+"/health", nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("health probe failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("server returned %d", resp.StatusCode)
    }

    return nil
}
```

**Session ID Mapping**:

```go
type SessionMapper struct {
    mu        sync.RWMutex
    mapping   map[string]string // opencodeID → agmSessionID
}

func (m *SessionMapper) Register(opencodeID, agmSessionID string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.mapping[opencodeID] = agmSessionID
}

func (m *SessionMapper) Lookup(opencodeID string) (string, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    agmID, ok := m.mapping[opencodeID]
    return agmID, ok
}
```

---

## Connection Resilience

### Auto-Reconnect Strategy

**Phases**:

1. **Initial Connection** (adapter start)
   - Attempt connection immediately
   - If fails: Log error, schedule reconnect with initial delay

2. **Connection Lost** (mid-session)
   - Detect via `scanner.Err()` or body close
   - Log disconnection event
   - Schedule reconnect with exponential backoff

3. **Reconnect Loop**
   - Wait `delay` (starts at 1s)
   - Attempt connection
   - On success: Reset delay, resume event processing
   - On failure: Multiply delay by 2, cap at 30s, retry

4. **Graceful Shutdown**
   - Cancel context
   - Break reconnect loop
   - Close HTTP connection

**Backoff Configuration**:

```yaml
reconnect:
  initial_delay: 1s   # First retry after 1 second
  max_delay: 30s      # Cap backoff at 30 seconds
  multiplier: 2       # Double delay on each failure
```

**Backoff Sequence**: 1s → 2s → 4s → 8s → 16s → 30s → 30s → ...

### Error Categories

| Error Type | Examples | Handling |
|------------|----------|----------|
| **Transient** | Network timeout, temporary DNS failure | Auto-reconnect |
| **Malformed Events** | Invalid JSON, missing fields | Log warning, skip event |
| **Server Unavailable** | 404, 503, connection refused | Exponential backoff |
| **Configuration Error** | Invalid server URL | Fatal, stop adapter |
| **EventBus Failure** | Publish timeout | Retry 3x, then log error |

### Fallback Strategy

If SSE adapter fails repeatedly:

```yaml
adapters:
  opencode:
    enabled: true
    fallback_to_tmux: true  # Astrocyte monitors if SSE fails
```

**Fallback Logic** (in daemon):

```go
func (d *Daemon) monitorAdapterHealth() {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        health := d.opencodeAdapter.Health()

        if !health.Connected && d.config.Adapters.OpenCode.FallbackToTmux {
            log.Warn("OpenCode SSE adapter unhealthy, Astrocyte will monitor")
            // Don't disable Astrocyte monitoring for this session
        }
    }
}
```

---

## EventBus Integration

### Event Schema

**Published Event**:

```go
type SessionStateChangeEvent struct {
    SessionID   string                 `json:"session_id"`
    State       string                 `json:"state"`
    Timestamp   int64                  `json:"timestamp"`
    Source      string                 `json:"source"`      // "opencode-sse"
    Agent       string                 `json:"agent"`       // "opencode"
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

**Example**:

```json
{
  "session_id": "my-opencode-session",
  "state": "AWAITING_PERMISSION",
  "timestamp": 1709654321,
  "source": "opencode-sse",
  "agent": "opencode",
  "metadata": {
    "event_type": "permission.asked",
    "permission_id": "perm-123",
    "action": "file.write",
    "path": "~/main.go"
  }
}
```

### State File Writing

**Important**: Adapter does NOT write state files directly. EventBus subscribers handle this.

**State File Writer** (daemon component):

```go
type StateFileWriter struct {
    eventBus   *eventbus.EventBus
    writeQueue chan StateWrite
}

type StateWrite struct {
    SessionID string
    State     string
    Timestamp int64
}

func (w *StateFileWriter) Start() {
    w.writeQueue = make(chan StateWrite, 1000)

    // EventBus subscriber (non-blocking)
    w.eventBus.Subscribe(eventbus.EventSessionStateChange, w.enqueueWrite)

    // Async writer goroutine
    go w.processWrites()
}

func (w *StateFileWriter) enqueueWrite(event eventbus.Event) {
    payload := event.Payload.(eventbus.SessionStateChangeEvent)

    select {
    case w.writeQueue <- StateWrite{
        SessionID: payload.SessionID,
        State:     payload.State,
        Timestamp: payload.Timestamp,
    }:
        // Enqueued successfully
    default:
        // Queue full, log warning but don't block EventBus
        log.Warn("State file write queue full, dropping write for %s", payload.SessionID)
        metrics.Inc("state_file_writes_dropped")
    }
}

func (w *StateFileWriter) processWrites() {
    for write := range w.writeQueue {
        path := filepath.Join(
            os.Getenv("HOME"),
            ".agm/sessions",
            write.SessionID,
            "state",
        )

        content := fmt.Sprintf("%s %d", write.State, write.Timestamp)

        if err := os.WriteFile(path, []byte(content), 0644); err != nil {
            log.Error("Failed to write state file: %v", err)
            metrics.Inc("state_file_write_errors")
        }
    }
}
```

**State File Format** (v4):

```
THINKING 1709654321
```

---

## Configuration

### Schema

```go
type Config struct {
    Enabled       bool          `yaml:"enabled"`
    ServerURL     string        `yaml:"server_url"`
    SessionID     string        `yaml:"-"` // Loaded from manifest
    Reconnect     ReconnectCfg  `yaml:"reconnect"`
    FallbackTmux  bool          `yaml:"fallback_to_tmux"`
}

type ReconnectCfg struct {
    InitialDelay  time.Duration `yaml:"initial_delay"`
    MaxDelay      time.Duration `yaml:"max_delay"`
    Multiplier    int           `yaml:"multiplier"`
}
```

### YAML Example

```yaml
adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2
    fallback_to_tmux: true
```

### Environment Overrides

```bash
export OPENCODE_SERVER_URL="http://localhost:8080"
export OPENCODE_ADAPTER_ENABLED="true"
```

---

## Session Lifecycle

### Session Creation

```
1. User: agm session new my-session --harness opencode-cli -C /path/to/project
2. AGM: Creates manifest.json with agent: "opencode"
3. Daemon: Detects new manifest (inotify watch)
4. Daemon: Starts OpenCode SSE adapter for this session
5. Adapter: Connects to http://localhost:4096/event
6. OpenCode: Sends session.created event
7. Adapter: Publishes READY state to EventBus
8. Daemon: Writes state file
```

### Session Termination

```
1. User: Exits OpenCode TUI (opencode attach)
2. OpenCode: Sends session.closed event via SSE
3. Adapter: Publishes TERMINATED state to EventBus
4. Daemon: Updates state file
5. Adapter: Unsubscribes from SSE stream
6. Daemon: Cleans up adapter instance
```

### Manifest Integration

**Session Manifest** (`~/.agm/sessions/{id}/manifest.json`):

```json
{
  "session_id": "my-opencode-session",
  "agent": "opencode",
  "created_at": "2026-03-06T14:23:00Z",
  "working_dir": "~/project",
  "tmux": {
    "session": "my-opencode-session",
    "window": 0,
    "pane": 0
  },
  "opencode": {
    "server_url": "http://localhost:4096",
    "session_id": "abc-123-def"
  }
}
```

**Adapter Initialization**:

```go
func (d *Daemon) startOpenCodeAdapter(manifest *Manifest) error {
    config := Config{
        Enabled:      d.config.Adapters.OpenCode.Enabled,
        ServerURL:    manifest.OpenCode.ServerURL,
        SessionID:    manifest.SessionID,
        Reconnect:    d.config.Adapters.OpenCode.Reconnect,
        FallbackTmux: d.config.Adapters.OpenCode.FallbackToTmux,
    }

    adapter := NewAdapter(d.eventBus, config)
    return adapter.Start(d.ctx)
}
```

---

## Testing Strategy

### Unit Tests

**SSE Client Tests** (`sse_adapter_test.go`):

```go
func TestSSEClient_Connect(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "data: {\"type\":\"session.created\"}\n\n")
    }))
    defer server.Close()

    adapter := NewSSEAdapter(mockEventBus, Config{ServerURL: server.URL})
    err := adapter.connect()
    require.NoError(t, err)
    assert.True(t, adapter.connected.Load())
}

func TestSSEClient_Reconnect(t *testing.T) {
    callCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        if callCount < 3 {
            // Fail first 2 attempts
            w.WriteHeader(http.StatusServiceUnavailable)
            return
        }
        // Succeed on 3rd attempt
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    adapter := NewSSEAdapter(mockEventBus, Config{
        ServerURL: server.URL,
        Reconnect: ReconnectCfg{InitialDelay: 10 * time.Millisecond},
    })
    adapter.Start(context.Background())

    // Wait for reconnect
    time.Sleep(100 * time.Millisecond)
    assert.Equal(t, 3, callCount)
    assert.True(t, adapter.connected.Load())
}
```

**Event Parser Tests** (`event_parser_test.go`):

```go
func TestEventParser_MapPermissionAsked(t *testing.T) {
    parser := NewEventParser()
    data := []byte(`{
        "type": "permission.asked",
        "timestamp": 1709654321,
        "properties": {"permission": {"id": "perm-123"}}
    }`)

    event, err := parser.Parse(data)
    require.NoError(t, err)
    assert.Equal(t, "AWAITING_PERMISSION", event.State)
    assert.Equal(t, int64(1709654321), event.Timestamp)
}

func TestEventParser_MalformedJSON(t *testing.T) {
    parser := NewEventParser()
    data := []byte(`{invalid json`)

    _, err := parser.Parse(data)
    assert.Error(t, err)
}
```

**Publisher Tests** (`publisher_test.go`):

```go
func TestPublisher_PublishToEventBus(t *testing.T) {
    mockBus := &mockEventBus{}
    publisher := NewPublisher(mockBus, "test-session")

    event := &AGMEvent{
        State:     "THINKING",
        Timestamp: 1709654321,
    }

    err := publisher.Publish(event)
    require.NoError(t, err)

    assert.Equal(t, 1, len(mockBus.published))
    assert.Equal(t, "test-session", mockBus.published[0].SessionID)
    assert.Equal(t, "THINKING", mockBus.published[0].State)
}
```

### Integration Tests

**Mock SSE Server** (`integration_test.go`):

```go
func TestE2E_SSEToStateFile(t *testing.T) {
    // Setup mock OpenCode SSE server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(http.StatusOK)

        // Send events
        fmt.Fprintf(w, "data: {\"type\":\"session.created\",\"timestamp\":1709654321}\n\n")
        time.Sleep(100 * time.Millisecond)
        fmt.Fprintf(w, "data: {\"type\":\"tool.execute.before\",\"timestamp\":1709654322}\n\n")
        time.Sleep(100 * time.Millisecond)
        fmt.Fprintf(w, "data: {\"type\":\"tool.execute.after\",\"timestamp\":1709654325}\n\n")
    }))
    defer server.Close()

    // Create adapter
    eventBus := eventbus.New()
    stateWriter := NewStateFileWriter(eventBus, t.TempDir())
    stateWriter.Start()

    adapter := NewAdapter(eventBus, Config{ServerURL: server.URL, SessionID: "test"})
    adapter.Start(context.Background())

    // Wait for events to process
    time.Sleep(500 * time.Millisecond)

    // Verify state file written
    statePath := filepath.Join(t.TempDir(), "test", "state")
    content, err := os.ReadFile(statePath)
    require.NoError(t, err)
    assert.Contains(t, string(content), "IDLE 1709654325")
}
```

### E2E Tests (Manual)

**Test Plan**:

1. Start real OpenCode server: `opencode serve --port 4096`
2. Create AGM session: `agm session new test --harness opencode-cli`
3. Send prompt: `agm send test "write hello world to main.go"`
4. Verify state transitions:
   - `cat ~/.agm/sessions/test/state` shows `IDLE → THINKING → IDLE`
5. Check notifications triggered
6. Verify tmux status line updates

**Chaos Tests**:

1. **Network Failure**: Disconnect mid-session
   - Kill OpenCode server: `pkill -9 opencode`
   - Verify adapter logs reconnect attempts
   - Restart server: `opencode serve --port 4096`
   - Verify adapter reconnects successfully

2. **Malformed Events**: Send invalid JSON
   - Mock server sends `data: {invalid`
   - Verify adapter logs warning but continues

3. **EventBus Overflow**: Send 10,000 events rapidly
   - Verify no events dropped
   - Verify queue depth metric stays < 1000

---

## Performance Characteristics

### Latency

| Operation | Expected Latency | Notes |
|-----------|-----------------|-------|
| SSE Connection | 50-100ms | Initial HTTP handshake |
| Event Parsing | <1ms | JSON unmarshal |
| EventBus Publish | <5ms | In-memory channel |
| State File Write | <10ms | Disk I/O |
| **Total E2E** | **100-200ms** | OpenCode event → state file |

### Resource Usage

| Resource | Per Adapter | Notes |
|----------|------------|-------|
| Memory | ~50KB | HTTP client + buffers |
| CPU | <1% | Mostly I/O wait |
| Network | ~1KB/event | Minimal overhead |
| Goroutines | 3 | Connection, reader, health |

### Scalability

- **Concurrent Sessions**: One adapter instance per OpenCode server
- **Event Throughput**: ~1000 events/sec (limited by EventBus buffer)
- **Connection Pooling**: Not needed (single server per adapter)

---

## Observability

### Metrics (Prometheus)

```prometheus
# Connection status
agm_opencode_adapter_connected{server="localhost:4096"} 1

# Event counts
agm_opencode_events_total{type="permission.asked"} 42
agm_opencode_events_total{type="tool.execute.before"} 128

# Error counts
agm_opencode_events_errors_total{reason="malformed_json"} 3
agm_opencode_events_errors_total{reason="validation_failed"} 1

# Reconnect tracking
agm_opencode_reconnects_total 5

# Latency
agm_opencode_event_processing_duration_seconds 0.002
```

### Logging

```
[INFO] OpenCode SSE adapter started (server: http://localhost:4096)
[INFO] Connected to OpenCode SSE endpoint
[INFO] Received event: permission.asked (session: my-session)
[INFO] Published AWAITING_PERMISSION state to EventBus
[WARN] SSE connection lost, reconnecting in 2s...
[INFO] Reconnected successfully (attempt 3)
[ERROR] Failed to parse event: invalid JSON
[ERROR] EventBus publish failed: buffer full
```

### Health Checks

**agm status output**:

```bash
$ agm status

Sessions: 1 active

Adapters:
  opencode-sse: ✓ Connected (last event: 2s ago)
    Server: http://localhost:4096
    Events: 247 total, 0 errors
    Uptime: 2h 15m

EventBus:
  ✓ Running (queue depth: 5/1000)
```

---

## Error Handling

### Connection Errors

```go
func (a *SSEAdapter) handleConnectionError(err error) error {
    if isTransient(err) {
        log.Warn("Transient error: %v, will retry", err)
        metrics.Inc("opencode.errors.transient")
        return nil // Trigger reconnect
    }

    if isConfigurationError(err) {
        log.Error("Configuration error: %v, stopping adapter", err)
        metrics.Inc("opencode.errors.fatal")
        a.Stop(context.Background())
        return err
    }

    log.Warn("Connection error: %v, will retry", err)
    return nil
}

func isTransient(err error) bool {
    if err == nil {
        return false
    }
    // Network timeouts, temporary DNS failures
    return strings.Contains(err.Error(), "timeout") ||
           strings.Contains(err.Error(), "temporary")
}
```

### Event Processing Errors

```go
func (a *SSEAdapter) handleEvent(data []byte) {
    // Parse event
    agmEvent, err := a.parser.Parse(data)
    if err != nil {
        log.Warn("Failed to parse event: %v", err)
        metrics.Inc("opencode.events.parse_errors")
        return // Skip this event, continue processing
    }

    // Publish to EventBus
    if err := a.publisher.Publish(agmEvent); err != nil {
        log.Error("Failed to publish event: %v", err)
        metrics.Inc("opencode.events.publish_errors")
        // Could implement retry logic here
    }

    a.lastEvent.Store(time.Now())
    metrics.Inc("opencode.events.processed")
}
```

---

## Integration with Daemon

### Daemon Startup

```go
// internal/daemon/daemon.go
func (d *Daemon) Start() error {
    // ... existing setup ...

    // Start OpenCode adapters for all active sessions
    sessions, _ := d.listSessions()
    for _, session := range sessions {
        manifest, _ := d.loadManifest(session)

        if manifest.Agent == "opencode" && d.config.Adapters.OpenCode.Enabled {
            adapter := opencode.NewAdapter(d.eventBus, opencode.Config{
                ServerURL: manifest.OpenCode.ServerURL,
                SessionID: manifest.SessionID,
                Reconnect: d.config.Adapters.OpenCode.Reconnect,
            })

            if err := adapter.Start(d.ctx); err != nil {
                log.Warn("Failed to start OpenCode adapter for %s: %v", session, err)
                continue
            }

            d.adapters[session] = adapter
        }
    }

    return nil
}
```

### Daemon Shutdown

```go
func (d *Daemon) Stop() error {
    // Graceful shutdown with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    var wg sync.WaitGroup
    for sessionID, adapter := range d.adapters {
        wg.Add(1)
        go func(id string, a Adapter) {
            defer wg.Done()
            if err := a.Stop(ctx); err != nil {
                log.Warn("Adapter %s stop error: %v", id, err)
            }
        }(sessionID, adapter)
    }

    wg.Wait()
    return nil
}
```

---

## Future Enhancements

1. **Event Replay**: Persist SSE events to append-only log for replay after daemon restart
2. **Multi-Server Support**: One adapter instance managing multiple OpenCode servers
3. **WebSocket Fallback**: Use WebSocket if SSE not available (OpenCode future versions)
4. **Compression**: Support gzip-compressed SSE streams
5. **Authentication**: Support OpenCode auth tokens in HTTP headers

---

## References

- **MULTI-AGENT-INTEGRATION-SPEC.md**: Overall integration architecture
- **ADR-009**: EventBus as integration layer decision rationale
- **GEMINI-RESEARCH.md**: OpenCode SSE research findings
- **internal/eventbus/schema.go**: EventBus event schemas
- **internal/tui/eventbus_client.go**: WebSocket client pattern (similar to SSE)
- **SSE Specification**: https://html.spec.whatwg.org/multipage/server-sent-events.html

---

**End of Architecture Document**
