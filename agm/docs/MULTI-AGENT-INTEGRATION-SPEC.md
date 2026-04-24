# Multi-Agent Integration Specification

**Document Version**: 1.0
**Status**: Accepted, In Progress (Phases 0-2 Complete)
**Created**: 2026-03-06
**Last Updated**: 2026-03-07
**Bead**: oss-9k2z

---

## Overview

This specification defines how AGM (Agent Gateway Manager) supports multiple agent harnesses through a hybrid integration architecture. The solution leverages AGM v4's EventBus as the canonical integration layer and implements agent-specific adapters that publish state changes to the bus.

### Goals

1. **Enable multi-agent support** - Monitor Claude Code, Gemini CLI, OpenCode, and Cortex sessions
2. **Preserve proven reliability** - Keep Astrocyte Python tmux scraping for Claude/Gemini (0% failure rate)
3. **Eliminate scraping where possible** - Use native event streams (SSE, webhooks) when available
4. **Establish integration pattern** - EventBus as single source of truth for all state changes
5. **Enable extensibility** - Well-defined adapter pattern for future agents

### Success Criteria

- OpenCode sessions publish state changes via SSE adapter without tmux scraping
- AGM v4 notifications work automatically for all agent types
- Astrocyte continues monitoring Claude/Gemini sessions via tmux
- Architecture supports adding new agents with minimal changes to core
- All state changes flow through EventBus regardless of source

---

## Architecture

### Integration Layer: EventBus Hub

All state detection mechanisms publish to AGM's EventBus. Consumers subscribe to receive state changes without knowing the detection method.

```
                    ┌─────────────────────────────────────┐
                    │         AGM EventBus Hub            │
                    │  (Canonical integration layer)      │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────┼──────────────────────┐
                    │              │                      │
         ┌──────────▼────┐  ┌─────▼──────┐   ┌─────────▼────────┐
         │ OpenCode SSE  │  │  Gemini    │   │    Claude        │
         │   Adapter     │  │ Hooks/Tmux │   │     Tmux         │
         │ (Phase 2)     │  │ (Phase 4)  │   │  (Astrocyte)     │
         └───────────────┘  └────────────┘   └──────────────────┘
              │                   │                    │
              │              ┌────▼────────────────────▼────┐
              │              │   Astrocyte Python Daemon    │
              │              │   (tmux capture-pane)        │
              │              └──────────────┬───────────────┘
              │                             │
         ┌────▼──────┐               incidents.jsonl
         │ OpenCode  │                      │
         │  Server   │               ┌──────▼──────┐
         │ (SSE)     │               │  Astrocyte  │
         └───────────┘               │  Go Watcher │
                                     └─────────────┘
```

### State Flow

1. **Detection**: Adapter detects state change (SSE event, tmux scrape, hook callback)
2. **Publishing**: Adapter publishes event to EventBus with session ID and state
3. **State File**: EventBus subscriber writes `~/.agm/sessions/{id}/state` (v4 format)
4. **Consumption**: Notification manager, tmux status, Temporal workflows subscribe to events
5. **Display**: Tmux status line reads state file for ambient indicators

---

## Agent-Specific Strategies

### OpenCode: SSE Adapter (Phase 1-2)

**Detection Method**: Server-Sent Events from OpenCode server

**Architecture**:
- OpenCode runs as client-server: `opencode serve` (headless) + `opencode attach` (TUI)
- SSE endpoint: `GET /event` streams real-time events
- Events include: `permission.asked`, `tool.execute.before`, `tool.execute.after`, `session.created`

**Implementation**:
- Package: `internal/monitor/opencode/`
- SSE client with auto-reconnect and exponential backoff
- Event parser mapping OpenCode events to AGM states
- EventBus publisher for state transitions
- Session lifecycle manager for server start/stop detection

**State Mapping**:
| OpenCode Event | AGM State | Rationale |
|---------------|-----------|-----------|
| `permission.asked` | `AWAITING_PERMISSION` | User input required |
| `tool.execute.before` | `WORKING` | Tool execution in progress |
| `tool.execute.after` | `IDLE` | Tool completed, awaiting next action |
| `session.created` | `DONE` | Session initialized |
| `session.closed` | `TERMINATED` | Session ended |

**Fallback**: If SSE connection fails, Astrocyte can monitor via tmux as backup

**Configuration**:
```yaml
opencode:
  enabled: true
  server_url: "http://localhost:4096"
  reconnect:
    initial_delay: 1s
    max_delay: 30s
    multiplier: 2
  fallback_to_tmux: true
```

### Claude Code: Astrocyte Tmux (Current) + Optional Hooks (Phase 3)

**Primary Detection Method**: Tmux screen scraping via Astrocyte Python

**Architecture**:
- Astrocyte runs `tmux capture-pane` every 60 seconds
- Pattern matching for stuck states (mustering timeout, zero-token, cursor frozen)
- Writes incidents to `~/.agm/astrocyte/incidents.jsonl`
- Go watcher polls incidents and publishes to EventBus

**Why Keep Scraping**:
- Proven 0% failure rate in production (per ADR-0001)
- Detects emergent behaviors (stuck states, crashes) that hooks don't cover
- Resilient to Claude Code version changes

**Optional Enhancement: HTTP Webhooks**:
- Claude Code supports PreToolUse hook (synchronous HTTP POST)
- Could add local HTTP server to receive permission prompts in real-time
- **Deferred to Phase 6** (not critical, scraping already works)

**State Detection Patterns**:
- Mustering timeout: `>20 min with "Mustering..."`
- Zero-token bug: `0 tokens` displayed for >5 min
- Cursor frozen: No output change for >30 min
- Permission prompt: Detecting UI elements

### Gemini CLI: Astrocyte Tmux (Current) + Optional Hooks (Phase 4)

**Primary Detection Method**: Tmux screen scraping via Astrocyte Python

**Architecture**: Same as Claude Code (tmux scraping)

**Why Keep Scraping**:
- Headless mode (`--output-format json`) disables interactive TUI
- BeforeTool hook requires complex stdin/stdout proxy script
- Scraping is simpler and proven reliable

**Optional Enhancement: BeforeTool Hook**:
- Gemini CLI supports hooks via config file
- Hook receives JSON on stdin, responds via stdout
- Requires proxy script and socket communication
- **Deferred to Phase 4** (only if scraping proves unreliable)

### Cortex: TBD (Future Work)

**Strategy**: Evaluate integration options when work begins

**Recommended Approach**:
1. Check for native event streams (SSE, WebSocket, gRPC)
2. If available, build adapter similar to OpenCode
3. If not available, extend Astrocyte to monitor Cortex sessions
4. Follow adapter pattern established by OpenCode

---

## EventBus Integration

### Event Schema

All adapters publish events to EventBus using standard schema:

```go
type SessionStateChangeEvent struct {
    SessionID   string    `json:"session_id"`
    State       string    `json:"state"`        // IDLE, WORKING, AWAITING_PERMISSION, etc.
    Timestamp   int64     `json:"timestamp"`    // Unix seconds
    Sequence    uint64    `json:"sequence"`     // Monotonic sequence number (prevents race conditions)
    Source      string    `json:"source"`       // "opencode-sse", "astrocyte", "claude-hook"
    Agent       string    `json:"agent"`        // "opencode", "claude", "gemini"
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

**Event Types**:
- `EventSessionStateChange`: State transitions (IDLE → WORKING)
- `EventSessionCreated`: New session initialized
- `EventSessionTerminated`: Session ended
- `EventSessionEscalated`: Stuck state detected by Astrocyte

### State File Format (v4)

When state change event published, EventBus subscriber writes:

**Path**: `~/.agm/sessions/{session-id}/state`

**Format**: `{STATE} {TIMESTAMP}`

**Example**: `WORKING 1709654321`

**States**:
- `DONE`: Session initialized, awaiting first prompt
- `IDLE`: Agent waiting for user input
- `WORKING`: Agent thinking or executing tools
- `AWAITING_PERMISSION`: Permission prompt displayed
- `STUCK`: Astrocyte detected stuck state (mustering timeout, cursor frozen)
- `TERMINATED`: Session closed

**Why This Format**:
- Simple parsing for tmux status helper
- Timestamp enables staleness detection
- Declared as "tmux UI artifact" (persists across storage backend changes)

### Publisher Pattern

Adapters MUST NOT write state files directly. Instead:

1. Adapter publishes `SessionStateChangeEvent` to EventBus
2. EventBus subscriber (in daemon) writes state file
3. EventBus subscriber triggers notifications
4. EventBus subscriber updates internal state tracking

**Rationale**: Single code path for all state changes prevents inconsistencies

---

## Adapter Pattern

### Interface Contract

All adapters implement:

```go
type Adapter interface {
    // Start begins monitoring and publishes events to EventBus
    Start(ctx context.Context) error

    // Stop gracefully shuts down monitoring
    Stop(ctx context.Context) error

    // Health returns adapter status for diagnostics
    Health() HealthStatus

    // Name returns adapter identifier (e.g., "opencode-sse")
    Name() string
}

type HealthStatus struct {
    Connected bool
    Error     error
    LastEvent time.Time
    Metadata  map[string]interface{}
}
```

### Lifecycle Management

**Startup**:
1. Daemon reads config to determine enabled adapters
2. Daemon creates adapter instances
3. Daemon calls `Start()` on each adapter
4. Adapters begin monitoring and publishing to EventBus

**Shutdown**:
1. Daemon receives SIGTERM/SIGINT
2. Daemon calls `Stop()` on all adapters with context timeout
3. Adapters clean up resources (close connections, unsubscribe)
4. Daemon waits for graceful shutdown or timeout

**Health Monitoring**:
- Daemon periodically calls `Health()` on adapters
- Unhealthy adapters trigger warnings (logs, notifications)
- `agm status` command shows adapter health

### Error Handling

**Connection Failures**:
- Adapters MUST implement auto-reconnect with exponential backoff
- Log errors but DO NOT crash daemon
- Fallback to tmux scraping if configured

**Malformed Events**:
- Validate event schema before publishing to EventBus
- Log malformed events for debugging
- Discard invalid events (do not publish)

**Version Incompatibilities**:
- Detect agent version if possible (from event metadata)
- Log warnings if unexpected event types received
- Graceful degradation (ignore unknown events)

---

## Astrocyte Integration

### Agent Type Detection

Astrocyte Python must skip sessions monitored by native adapters:

**Implementation**:
1. Read `~/.agm/sessions/{id}/manifest.json`
2. Check `agent` field: `"claude"`, `"gemini"`, `"opencode"`, `"cortex"`
3. If `agent == "opencode"` and OpenCode adapter enabled: Skip monitoring
4. Otherwise: Continue tmux scraping

**Why Skip**:
- Prevents duplicate state change events
- OpenCode SSE provides better real-time detection than tmux scraping
- Reduces tmux load (fewer panes to scrape)

### Configuration Override

Allow forcing Astrocyte to monitor all sessions:

```yaml
astrocyte:
  force_scraping: false  # If true, ignore agent type and scrape everything
  skip_agents:
    - "opencode"         # Skip these agent types (default)
```

**Use Case**: Debugging, SSE adapter failure, OpenCode version incompatibility

### Logging

Astrocyte MUST log which sessions are skipped:

```
[2026-03-06 14:23:10] INFO: Skipping session my-session (agent: opencode, reason: SSE adapter enabled)
```

---

## Session Lifecycle

### Session Creation

**OpenCode**:
1. User runs `opencode serve --port 4096`
2. User runs `agm session new opencode-session --harness opencode-cli -C /path/to/project`
3. AGM creates manifest with `agent: "opencode"`
4. AGM daemon detects new session and starts SSE subscription
5. OpenCode SSE adapter connects to `http://localhost:4096/event`
6. Adapter publishes `SessionCreated` event to EventBus
7. AGM writes state file: `DONE {timestamp}`

**Claude/Gemini**:
1. User runs `agm session new my-session -C /path/to/project`
2. AGM creates manifest with `agent: "claude"` (default)
3. Astrocyte Python detects new manifest file (inotify watch)
4. Astrocyte begins scraping tmux pane
5. Astrocyte publishes incidents to `incidents.jsonl`
6. Go watcher publishes state changes to EventBus

### Session Termination

**OpenCode**:
1. User exits OpenCode TUI (`opencode attach`)
2. OpenCode server sends `session.closed` SSE event
3. Adapter publishes `SessionTerminated` event
4. AGM writes state file: `TERMINATED {timestamp}`
5. Adapter unsubscribes from SSE stream

**Claude/Gemini**:
1. User exits Claude session
2. Tmux pane closes or content changes
3. Astrocyte detects termination
4. Publishes `SessionTerminated` via incidents.jsonl
5. Go watcher publishes to EventBus

### Session Resurrection

If session re-opens (user resumes work):
1. Adapter detects new events (SSE reconnect, tmux pane reappears)
2. Publishes `SessionCreated` event
3. State file updated to `DONE` or `IDLE`

---

## Configuration Schema

### Daemon Configuration

**Path**: `~/.agm/config.yaml`

```yaml
# EventBus settings (v4)
eventbus:
  enabled: true
  buffer_size: 1000

# Notification manager (v4)
notifications:
  enabled: true
  backends:
    - "desktop"    # libnotify
    - "terminal"   # OSC 777

# Multi-agent adapters
adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2
    fallback_to_tmux: true

  claude_hooks:
    enabled: false  # Phase 3 (optional)
    listen_addr: "127.0.0.1:14321"

  gemini_hooks:
    enabled: false  # Phase 4 (optional, deferred)
    socket_path: "/tmp/agm-gemini-hook.sock"

# Astrocyte Python settings
astrocyte:
  enabled: true
  scan_interval: 60  # seconds
  force_scraping: false
  skip_agents:
    - "opencode"  # Skip if adapter enabled
```

### Session Manifest

**Path**: `~/.agm/sessions/{session-id}/manifest.json`

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

**Fields**:
- `agent`: Agent type (`"claude"`, `"gemini"`, `"opencode"`, `"cortex"`)
- `opencode.server_url`: For SSE adapter to connect
- `opencode.session_id`: OpenCode's internal session ID (if different from AGM session ID)

---

## State Transitions

### Valid Transitions

```
DONE → WORKING             (User sends prompt)
WORKING → IDLE             (Agent completes thinking)
WORKING → AWAITING_PERMISSION (Permission prompt displayed)
AWAITING_PERMISSION → WORKING (Permission granted)
AWAITING_PERMISSION → IDLE (Permission denied)
IDLE → WORKING             (User sends another prompt)
* → STUCK                 (Astrocyte detects stuck state)
STUCK → WORKING            (Recovery action taken)
* → TERMINATED            (Session closed)
```

### Invalid Transitions

Adapters SHOULD log warnings for unexpected transitions but MUST NOT block publishing:
- `IDLE → AWAITING_PERMISSION` (skip WORKING state)
- `DONE → AWAITING_PERMISSION` (permission before first prompt)

**Rationale**: Agent behavior evolves; graceful degradation better than crashes

---

## Testing Strategy

### Unit Tests

**SSE Adapter**:
- Connection management (connect, disconnect, reconnect)
- Event parsing (valid events, malformed JSON, unknown types)
- Error handling (network failures, timeouts)
- Backoff logic (exponential backoff, max retry)

**Event Parser**:
- OpenCode event → AGM state mapping
- Edge cases (missing fields, unexpected values)

**EventBus Publisher**:
- Event publishing (synchronous vs async)
- Error propagation
- Deduplication (same event published twice)

### Integration Tests

**Mock SSE Server**:
- Simulate OpenCode server sending events
- Test E2E flow: SSE → Adapter → EventBus → State File
- Verify state file contents and format
- Test reconnect on server restart

**Astrocyte Skipping**:
- Create OpenCode session
- Verify Astrocyte does NOT scrape pane
- Create Claude session
- Verify Astrocyte DOES scrape pane

### E2E Tests

**Real OpenCode Server**:
- Start `opencode serve`
- Create AGM session
- Send prompt via `agm send`
- Verify state transitions: IDLE → WORKING → IDLE
- Check notifications triggered
- Verify tmux status line updates

### Chaos Tests

**Network Failures**:
- Disconnect SSE mid-session
- Verify auto-reconnect
- Verify state transitions continue after reconnect

**Service Restarts**:
- Restart OpenCode server
- Verify adapter reconnects
- Verify session state preserved

**Concurrent Events**:
- Send multiple prompts rapidly
- Verify all state changes captured
- Verify no race conditions in EventBus

---

## Migration Path

### Phase 1: OpenCode SSE Adapter Implementation (Week 7-8)

**Deliverables**:
- `internal/monitor/opencode/sse_adapter.go`
- `internal/monitor/opencode/event_parser.go`
- `internal/monitor/opencode/publisher.go`
- `internal/monitor/opencode/lifecycle.go`
- Configuration schema in `internal/config/config.go`
- Unit tests with >80% coverage

### Phase 2: Daemon Integration (Week 9, Days 1-3)

**Deliverables**:
- Adapter startup in `internal/daemon/daemon.go`
- Health check endpoint
- Fallback to tmux scraping on SSE failure
- `agm status` command shows adapter health

### Phase 3: Astrocyte Modification (Week 9, Days 4-5)

**Deliverables**:
- Agent type detection in `astrocyte/astrocyte.py`
- Skip OpenCode sessions if SSE enabled
- Configuration flag to force scraping
- Debug logging for skipped sessions

### Phase 4: Testing (Week 10)

**Deliverables**:
- Unit test suite for SSE adapter
- Integration tests with mock server
- E2E tests with real OpenCode
- Chaos tests for network failures

### Phase 5: Documentation (Week 11, Days 1-3)

**Deliverables**:
- User guide: `docs/OPENCODE-INTEGRATION.md`
- Migration guide for existing users
- Updated `README.md` and `CHANGELOG.md`
- Example configuration files

---

## Backward Compatibility

### Existing Sessions

All existing Claude/Gemini sessions continue working:
- Astrocyte continues tmux scraping
- EventBus receives events via Go watcher
- State files written in v4 format
- Notifications work as before

**No breaking changes** for Claude/Gemini workflows.

### New Sessions

Users can opt-in to OpenCode monitoring:
```bash
agm session new my-session --harness opencode-cli -C /path/to/project
```

Default agent remains `claude` for backward compatibility.

### Configuration

If `adapters.opencode.enabled` not set, defaults to `false`:
- OpenCode sessions still work (monitored via Astrocyte)
- Users must explicitly enable SSE adapter

---

## Security Considerations

### SSE Connection

**Authentication**: OpenCode server runs on localhost (no authentication needed)

**Authorization**: SSE endpoint is read-only (no write operations)

**Network**: Adapter only connects to `localhost` (no remote servers)

**Risks**: If OpenCode server compromised, adapter receives malicious events

**Mitigation**: Validate all event schemas before publishing to EventBus

### HTTP Webhooks (Phase 3, Optional)

**Authentication**: Webhook server binds to `127.0.0.1` only (no remote access)

**Authorization**: Claude sends HMAC signature in webhook payload (validate before processing)

**Risks**: Local processes could spam webhook endpoint

**Mitigation**: Rate limiting, request size limits, signature validation

---

## Performance Considerations

### EventBus Queue Depth

**Risk**: High-frequency state changes could overflow EventBus buffer

**Monitoring**: Track `eventbus.queue_depth` metric

**Overflow Handling**:
- **SLO**: Queue depth >800 = warning, >950 = critical
- **Circuit Breaker**: After 10 consecutive publish failures, adapter stops and alerts
- **Backpressure**: Adapters pause reading events if publish fails 3 times with exponential backoff
- **Metrics**:
  - `agm_eventbus_queue_depth` (gauge)
  - `agm_eventbus_overflows_total` (counter)
  - `agm_eventbus_publish_failures_total{adapter="opencode-sse"}` (counter)

**Mitigation**:
- Increase buffer size if needed (default 1000, max 10000)
- Deduplication (don't publish identical consecutive states)
- Async state file writing (non-blocking EventBus subscribers)

**Example Backpressure Implementation**:
```go
func (p *Publisher) PublishWithBackpressure(event *AGMEvent) error {
    for i := 0; i < 3; i++ {
        err := p.eventBus.Publish(EventSessionStateChange, event)
        if err == nil {
            return nil
        }

        if errors.Is(err, ErrQueueFull) {
            metrics.Inc("eventbus.backpressure_delays")
            time.Sleep(100 * time.Millisecond * (1 << i)) // 100ms, 200ms, 400ms
            continue
        }
        return err
    }

    // Circuit breaker
    p.failureCount.Inc()
    if p.failureCount.Load() > 10 {
        log.Error("Circuit breaker open: stopping adapter")
        p.adapter.Stop(context.Background())
        return fmt.Errorf("circuit breaker open")
    }
    return fmt.Errorf("publish failed after 3 retries")
}
```

### SSE Connection Overhead

**Bandwidth**: SSE events are small (<1KB), minimal impact

**CPU**: Event parsing is lightweight (JSON unmarshaling)

**Memory**: One SSE client per OpenCode server (~50KB overhead)

### Astrocyte Load Reduction

Skipping OpenCode sessions reduces tmux load:
- Fewer panes to scrape every 60 seconds
- Lower CPU usage for `tmux capture-pane`

**Expected Reduction**: ~10-20% CPU if 50% of sessions are OpenCode

---

## Observability

### Metrics

Expose Prometheus metrics:
```
agm_adapter_connected{name="opencode-sse"} 1
agm_adapter_events_total{name="opencode-sse", event_type="permission.asked"} 42
agm_adapter_errors_total{name="opencode-sse", error_type="connection_failed"} 3
agm_adapter_reconnects_total{name="opencode-sse"} 5
agm_eventbus_events_published_total{source="opencode-sse"} 127
agm_eventbus_queue_depth 23
```

### Logging

**Adapter Lifecycle**:
```
[INFO] OpenCode SSE adapter started (server: http://localhost:4096)
[INFO] Connected to OpenCode SSE endpoint
[WARN] SSE connection lost, reconnecting in 2s...
[INFO] Reconnected to OpenCode SSE endpoint
[ERROR] Failed to parse event: invalid JSON
```

**State Changes**:
```
[INFO] Session my-session: IDLE → WORKING (source: opencode-sse)
[INFO] Session my-session: WORKING → AWAITING_PERMISSION (source: opencode-sse)
```

**Astrocyte Skipping**:
```
[INFO] Skipping session my-session (agent: opencode, reason: SSE adapter enabled)
```

### Diagnostics

**agm status command**:
```bash
$ agm status

Sessions: 3 active

Adapters:
  opencode-sse: ✓ Connected (last event: 2s ago)
  claude-hooks: ✗ Disabled
  gemini-hooks: ✗ Disabled

Astrocyte:
  ✓ Running (monitoring 2 sessions, skipping 1)

EventBus:
  ✓ Running (queue depth: 5/1000)
```

---

## Design Decisions

1. **OpenCode server discovery** (DECIDED)
   - **Decision**: Server URL provided in session manifest (`manifest.opencode.server_url`)
   - Health probe on adapter start confirms server reachable
   - If probe fails, adapter logs error and uses fallback (Astrocyte tmux monitoring)
   - **Rationale**: Simple, explicit configuration; user controls server lifecycle

2. **Multi-server OpenCode** (DECIDED)
   - **Decision**: One adapter instance per OpenCode server
   - Each session manifest specifies its server URL
   - Daemon manages adapter lifecycle per session
   - **Alternative Considered**: Single adapter with connection pooling (rejected: added complexity)

3. **State file ownership** (DECIDED)
   - **Decision**: Adapters ONLY publish events; EventBus subscriber writes files
   - **Rationale**: Single code path prevents inconsistencies and race conditions

4. **Cortex integration timeline** (DEFERRED)
   - **Decision**: Evaluate when work begins
   - Architecture designed for extensibility (adapter pattern)

---

## References

- **AGM v4 Spec**: `research/sessions/2026-03-06-agm-v4-spec-review-8OX1U/AGM-V4-SPEC.md`
- **Gemini Research**: `research/sessions/2026-03-06-agm-multi-agent-integration/GEMINI-RESEARCH.md`
- **Analysis Document**: `research/sessions/2026-03-06-agm-multi-agent-integration/ANALYSIS.md`
- **ADR-0001**: `main/agm/internal/tmux/ADR-0001-capture-pane-vs-control-mode.md`
- **ADR-007**: `main/agm/docs/adr/ADR-007-hook-based-state-detection.md`
- **OpenCode GitHub**: https://github.com/anomaly/opencode
- **SSE Specification**: https://html.spec.whatwg.org/multipage/server-sent-events.html

---

## Appendix A: State Diagram

```
                    ┌─────────┐
                    │  DONE   │ (Session initialized)
                    └────┬────┘
                         │
                    User sends prompt
                         │
                         ▼
    ┌────────────────►┌──────┐◄────────────────┐
    │                 │ WORKING │ (Thinking)       │
    │                 └───┬──┘                  │
    │                     │                     │
    │            ┌────────┼─────────┐           │
    │            │                  │           │
    │     Tool complete     Permission needed   │
    │            │                  │           │
    │            ▼                  ▼           │
    │        ┌──────┐     ┌────────────────────┐│
    └────────│ IDLE │     │ AWAITING_PERMISSION││
             └──────┘     └────────────────────┘│
                │                   │            │
                │         Permission granted     │
                │                   └────────────┘
                │
                │ (Any state can transition to STUCK or TERMINATED)
                │
                ▼
            ┌────────────┐
            │ TERMINATED │
            └────────────┘
```

---

## Appendix B: Example SSE Event

**OpenCode Event Stream** (`GET /event`):

```
data: {"type": "permission.asked", "timestamp": 1709654321, "properties": {"permission": {"id": "perm-123", "action": "file.write", "path": "~/main.go"}}}

data: {"type": "tool.execute.before", "timestamp": 1709654322, "properties": {"tool": "Write", "args": {"file_path": "~/main.go"}}}

data: {"type": "tool.execute.after", "timestamp": 1709654325, "properties": {"tool": "Write", "success": true}}
```

**AGM EventBus Event** (after parsing):

```json
{
  "session_id": "my-opencode-session",
  "state": "AWAITING_PERMISSION",
  "timestamp": 1709654321,
  "source": "opencode-sse",
  "agent": "opencode",
  "metadata": {
    "permission_id": "perm-123",
    "action": "file.write",
    "path": "~/main.go"
  }
}
```

---

**End of Specification**
