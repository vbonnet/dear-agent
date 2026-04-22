# ADR-009: EventBus as Multi-Agent Integration Layer

## Status
**Accepted** (2026-03-06)

## Context

AGM needs to support multiple agent harnesses (Claude Code, Gemini CLI, OpenCode, Cortex) with different state detection mechanisms:
- **Claude Code**: tmux screen scraping via Astrocyte Python (proven 0% failure rate)
- **Gemini CLI**: tmux screen scraping via Astrocyte Python (same mechanism)
- **OpenCode**: Native SSE event stream at `/event` endpoint
- **GitHub Copilot CLI**: JSON-RPC telemetry (if adopted)
- **Cortex**: To be determined

**Requirements:**
- Support multiple state detection methods simultaneously
- Preserve existing Astrocyte Python tmux scraping (proven reliable)
- Eliminate screen scraping where native events available (OpenCode)
- Single integration point for all consumers (notifications, status, Temporal)
- Enable adding new agents without breaking existing functionality
- No code duplication across adapters

**Constraints:**
- Cannot replace Astrocyte Python (6+ months of production heuristics)
- Must work with AGM v4 notification manager (already uses EventBus)
- State file format defined by v4 (`WORKING 1709654321`)
- Existing consumers must continue working without changes

## Decision

**Use AGM v4's EventBus as the canonical integration layer for all agent state changes.**

### Architecture

All state detection mechanisms (tmux scraping, SSE streams, HTTP webhooks, etc.) publish to a central EventBus. Consumers subscribe once to receive state changes from all agents.

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
         │   Adapter     │  │    Tmux    │   │     Tmux         │
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

**Key Principle**: EventBus is the source of truth. State files, notifications, and metrics are all derived from EventBus events.

### Event Schema

```go
type SessionStateChangeEvent struct {
    SessionID   string                 `json:"session_id"`
    State       string                 `json:"state"`        // IDLE, WORKING, etc.
    Timestamp   int64                  `json:"timestamp"`    // Unix seconds
    Source      string                 `json:"source"`       // "opencode-sse", "astrocyte"
    Agent       string                 `json:"agent"`        // "opencode", "claude", "gemini"
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

### Adapter Pattern

All adapters implement:

```go
type Adapter interface {
    Start(ctx context.Context) error  // Begin monitoring
    Stop(ctx context.Context) error   // Graceful shutdown
    Health() HealthStatus             // Diagnostics
    Name() string                     // Identifier
}
```

Adapters publish to EventBus:

```go
func (a *OpenCodeAdapter) handleEvent(event OpenCodeEvent) {
    agmEvent := SessionStateChangeEvent{
        SessionID: a.sessionID,
        State:     mapOpenCodeStateToAGM(event),
        Timestamp: time.Now().Unix(),
        Source:    "opencode-sse",
        Agent:     "opencode",
    }

    a.eventBus.Publish(EventSessionStateChange, agmEvent)
}
```

### Consumer Pattern

Consumers subscribe once to EventBus:

```go
// Notification manager (v4)
func (m *Manager) Start() {
    m.eventBus.Subscribe(EventSessionStateChange, m.handleStateChange)
}

func (m *Manager) handleStateChange(event Event) {
    payload := event.Payload.(SessionStateChangeEvent)

    // Write state file for tmux status
    writeStateFile(payload.SessionID, payload.State, payload.Timestamp)

    // Trigger notifications if needed
    if payload.State == "STUCK" {
        m.sendNotification(payload)
    }
}
```

## Alternatives Considered

### 1. Per-Adapter HTTP Servers

**Implementation:** Each adapter runs separate HTTP server, consumers poll all servers

```
OpenCode Adapter → HTTP :14321
Gemini Adapter   → HTTP :14322
Claude Adapter   → HTTP :14323
                    ↓
Notification Manager polls all 3 endpoints
```

**Pros:** Decoupled adapters, language-agnostic
**Cons:**
- Complex state reconciliation (which server is authoritative?)
- Code duplication (each consumer implements HTTP client)
- Race conditions (state changes during polling)
- N×M complexity (N adapters × M consumers)

**Rejected:** EventBus already solves pub/sub problem, no need for HTTP layer

### 2. Shared Database (PostgreSQL, SQLite)

**Implementation:** Adapters write to `session_states` table, consumers query

**Pros:** ACID transactions, SQL queries
**Cons:**
- Polling required (no push notifications)
- Database becomes single point of failure
- Slower than in-memory EventBus
- Requires schema migrations

**Rejected:** Over-engineered, polling defeats purpose of real-time events

### 3. Message Queue (RabbitMQ, Redis Pub/Sub)

**Implementation:** External message broker, adapters publish, consumers subscribe

**Pros:** Battle-tested, scales to distributed systems
**Cons:**
- External dependency (another service to run)
- Overkill for single-machine use case
- Network overhead for localhost communication
- Requires authentication/ACLs

**Rejected:** AGM v4 already has in-process EventBus, no need for external broker

### 4. File Tailing (inotify on state files)

**Implementation:** Each adapter writes state file, consumers watch with inotify

**Pros:** Simple, no EventBus
**Cons:**
- Race conditions (read file mid-write)
- inotify limits (max 8192 watches)
- File I/O overhead for every state change
- No atomic multi-consumer delivery

**Rejected:** Fragile, doesn't scale, EventBus is more reliable

### 5. Replace Astrocyte Python with Go

**Implementation:** Rewrite all tmux scraping logic in Go, integrate with EventBus

**Pros:** Single language, simpler architecture
**Cons:**
- Loses 6+ months of production-proven heuristics
- Python iteration speed critical for detection patterns
- Risk of introducing regressions
- No clear benefit (EventBus already bridges Python → Go)

**Rejected:** Don't fix what isn't broken (ADR-0001: capture-pane has 0% failure rate)

## Consequences

### Positive

✅ **Single integration point**: Consumers subscribe to EventBus, not each adapter
✅ **No code duplication**: State file writing, notifications, metrics all in one place
✅ **Decoupled adapters**: OpenCode SSE, Claude hooks, Gemini stdin/stdout all publish same event schema
✅ **Proven infrastructure**: EventBus already used by v4 notification manager
✅ **Incremental adoption**: Add adapters without breaking existing consumers
✅ **Language-agnostic**: Python (Astrocyte) and Go (daemon) both publish to same bus
✅ **Real-time**: Push-based (no polling), state changes propagate immediately
✅ **Testable**: Mock EventBus for unit tests, no external dependencies

### Negative

❌ **EventBus dependency**: All components depend on EventBus being available
❌ **In-process only**: Doesn't support distributed AGM (future constraint)
❌ **Queue overflow risk**: High-frequency state changes could overflow buffer

### Neutral

🔵 **Queue depth monitoring**: Need metrics to detect overflow (mitigated by 1000-event buffer)
🔵 **Adapter failures**: Need health checks and fallback logic
🔵 **Event ordering**: Events from different adapters may interleave (acceptable)

## Implementation

### Phase 1: OpenCode SSE Adapter (Week 7-8)

**Package**: `internal/monitor/opencode/`

**Files**:
- `sse_adapter.go` (SSE client, auto-reconnect)
- `event_parser.go` (OpenCode → AGM state mapping)
- `publisher.go` (EventBus publishing)
- `lifecycle.go` (Session start/stop detection)

**Integration**:
```go
// internal/daemon/daemon.go
func (d *Daemon) Start() error {
    // Existing v4 setup
    d.eventBus = eventbus.New()
    d.notificationManager = notifications.NewManager(d.eventBus)

    // NEW: Start OpenCode adapter if enabled
    if d.config.Adapters.OpenCode.Enabled {
        adapter := opencode.NewAdapter(d.eventBus, d.config.Adapters.OpenCode)
        d.adapters = append(d.adapters, adapter)
        adapter.Start(d.ctx)
    }

    // Existing Astrocyte watcher
    d.astrocyteWatcher.Start(d.ctx)
}
```

### Phase 2: Astrocyte Agent Detection (Week 9, Days 4-5)

**Modification**: `astrocyte/astrocyte.py`

```python
def should_monitor_session(manifest):
    """Skip sessions with native adapters enabled."""
    agent = manifest.get("agent", "claude")

    # Check if adapter enabled in AGM config
    config = load_agm_config()

    if agent == "opencode" and config.adapters.opencode.enabled:
        log.info(f"Skipping {manifest['session_id']} (opencode adapter enabled)")
        return False

    return True
```

### EventBus Subscription (Consumers)

**Notification Manager** (already implemented in v4):
```go
func (m *Manager) Start() {
    m.eventBus.Subscribe(EventSessionStateChange, m.handleStateChange)
    m.eventBus.Subscribe(EventSessionEscalated, m.handleEscalation)
}
```

**State File Writer** (new component):
```go
type StateFileWriter struct {
    eventBus *eventbus.EventBus
}

func (w *StateFileWriter) Start() {
    w.eventBus.Subscribe(EventSessionStateChange, w.writeStateFile)
}

func (w *StateFileWriter) writeStateFile(event Event) {
    payload := event.Payload.(SessionStateChangeEvent)
    path := fmt.Sprintf("~/.agm/sessions/%s/state", payload.SessionID)
    content := fmt.Sprintf("%s %d", payload.State, payload.Timestamp)
    os.WriteFile(path, []byte(content), 0644)
}
```

## Validation

### Functional Tests

- ✅ OpenCode SSE events published to EventBus
- ✅ Astrocyte incidents published to EventBus
- ✅ Multiple consumers receive same event
- ✅ State files written for all agent types
- ✅ Notifications triggered for all agent types

### Integration Tests

- ✅ Mock OpenCode SSE server → EventBus → state file written
- ✅ Astrocyte JSONL → EventBus → notification sent
- ✅ Concurrent events from multiple adapters processed correctly

### E2E Tests

- ✅ Real OpenCode session: SSE → EventBus → tmux status updates
- ✅ Real Claude session: tmux scraping → EventBus → notification
- ✅ Mixed session types: all monitored without conflicts

### Chaos Tests

- ✅ EventBus buffer overflow: verify graceful degradation
- ✅ Adapter crash: verify other adapters unaffected
- ✅ Consumer crash: verify EventBus continues publishing

## Related Decisions

- **ADR-006**: Message Queue Architecture (uses EventBus for queue management)
- **ADR-007**: Hook-Based State Detection (hooks publish to EventBus)
- **ADR-008**: Status Aggregation (queries EventBus for current states)
- **AGM v4 Spec**: Establishes EventBus as integration layer (Feature 1: Notifications)

## Migration Notes

**Backward Compatibility:**
- Existing Claude/Gemini sessions continue working (Astrocyte unchanged)
- EventBus already exists (v4), no new infrastructure
- State file format unchanged (`WORKING 1709654321`)
- Consumers already subscribe to EventBus (v4 notification manager)

**Incremental Rollout:**
1. Phase 1: Add OpenCode adapter (new code, no changes to existing)
2. Phase 2: Modify Astrocyte to skip OpenCode sessions (optional, has fallback)
3. Phase 3+: Add Claude hooks, Gemini hooks (optional enhancements)

**No Breaking Changes**: All new functionality is additive.

**Fallback Strategy:**
If OpenCode adapter fails, Astrocyte can monitor via tmux:
```yaml
adapters:
  opencode:
    enabled: true
    fallback_to_tmux: true  # Astrocyte monitors if SSE fails
```

## Future Enhancements

**Distributed AGM** (out of scope):
- Replace in-process EventBus with Redis Pub/Sub or NATS
- Same adapter code, different EventBus backend
- Enables multi-machine AGM clusters

**Event Persistence**:
- Write EventBus events to append-only log (audit trail)
- Replay events after daemon restart
- Historical state queries

**Webhook Support**:
- Expose EventBus events via HTTP webhooks
- External systems subscribe to AGM state changes
- Enables Slack/Discord/PagerDuty integrations

---

**Deciders:** Foundation Engineering
**Date:** 2026-03-06
**Implementation:** Phase 1-2 (Weeks 7-9)
**Bead:** oss-5m41 (in progress)
