# Astrocyte EventBus Integration

This package provides integration between Astrocyte (AGM's session monitoring daemon) and the WebSocket EventBus for real-time escalation tracking.

## Overview

Astrocyte monitors AGM sessions for stuck states (permission prompts, zero-token galloping, API stalls, etc.) and logs incidents to `~/.agm/astrocyte/incidents.jsonl`. This integration:

1. **Watches** the Astrocyte incidents file for new entries
2. **Publishes** escalation events to the WebSocket EventBus
3. **Prevents duplicates** using time-windowed deduplication (15-minute window by default)

## Architecture

```
┌─────────────────┐
│   Astrocyte     │  (Python daemon)
│    Daemon       │
└────────┬────────┘
         │ writes
         ▼
┌─────────────────────┐
│  incidents.jsonl    │
└────────┬────────────┘
         │ watches
         ▼
┌─────────────────────┐
│  Watcher (Go)       │
│  - Polls file       │
│  - Parses JSONL     │
│  - Time-windows     │
└────────┬────────────┘
         │ broadcasts
         ▼
┌─────────────────────┐
│   EventBus Hub      │
│   (WebSocket)       │
└────────┬────────────┘
         │
         ▼
┌─────────────────────┐
│  WebSocket Clients  │
│  (Temporal, UIs)    │
└─────────────────────┘
```

## Components

### Watcher

Monitors the Astrocyte incidents file and publishes events:

```go
watcher := astrocyte.NewWatcher(hub, incidentsFile, escalationWindow)
watcher.Start()
defer watcher.Stop()
```

**Parameters:**
- `hub`: EventBus Hub instance
- `incidentsFile`: Path to incidents.jsonl (empty string uses default)
- `escalationWindow`: Time window for duplicate prevention (0 uses 15 minutes)

### EscalationTracker

Thread-safe tracker for preventing duplicate escalations within a time window:

```go
tracker := astrocyte.NewEscalationTracker(15 * time.Minute)

if tracker.ShouldPublish(sessionID) {
    // Publish event
    hub.Broadcast(event)
    tracker.RecordEscalation(sessionID)
}
```

**Features:**
- Concurrent-safe (`sync.Map`)
- Per-session tracking
- Configurable time window

## Integration

### Basic Integration

```go
import (
    "github.com/vbonnet/dear-agent/agm/internal/astrocyte"
    "github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// 1. Create EventBus
hub := eventbus.NewHub()
go hub.Run()

// 2. Start WebSocket server
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    eventbus.ServeWebSocket(hub, w, r)
})
go http.ListenAndServe(":8080", nil)

// 3. Start Astrocyte watcher
watcher, err := astrocyte.StartAstrocyteMonitoring(hub, "", 15*time.Minute)
if err != nil {
    log.Fatal(err)
}
defer watcher.Stop()
```

### Integration with AGM Daemon

Add to `internal/daemon/daemon.go`:

```go
type Daemon struct {
    // ... existing fields
    eventHub        *eventbus.Hub
    astrocyteWatcher *astrocyte.Watcher
}

func (d *Daemon) Start() error {
    // ... existing initialization

    // Start EventBus
    d.eventHub = eventbus.NewHub()
    go d.eventHub.Run()

    // Start WebSocket server
    http.HandleFunc("/ws/events", func(w http.ResponseWriter, r *http.Request) {
        eventbus.ServeWebSocket(d.eventHub, w, r)
    })

    // Start Astrocyte watcher
    watcher, err := astrocyte.StartAstrocyteMonitoring(d.eventHub, "", 15*time.Minute)
    if err != nil {
        log.Printf("⚠️  Failed to start Astrocyte monitoring: %v", err)
    } else {
        d.astrocyteWatcher = watcher
    }

    // ... rest of daemon startup
}

func (d *Daemon) Shutdown() {
    if d.astrocyteWatcher != nil {
        d.astrocyteWatcher.Stop()
    }
    if d.eventHub != nil {
        d.eventHub.Shutdown()
    }
}
```

## Event Schema

Escalation events published to the EventBus:

```json
{
  "type": "session.escalated",
  "timestamp": "2026-02-14T12:30:00Z",
  "session_id": "my-session",
  "payload": {
    "escalation_type": "error",
    "pattern": "mustering_timeout",
    "line": "✻ Mustering...",
    "line_number": 0,
    "detected_at": "2026-02-14T12:30:00Z",
    "description": "Session stuck: stuck_mustering detected (duration: 20 min), recovery: escape (success)",
    "severity": "high"
  }
}
```

### Escalation Types

| Astrocyte Symptom | Escalation Type | Severity |
|-------------------|-----------------|----------|
| `stuck_mustering` | `error` | `high` |
| `stuck_waiting` | `error` | `high` |
| `cursor_frozen` | `error` | `high` |
| `permission_prompt` | `prompt` | `medium` |
| `ask_question_violation` | `warning` | `low` |
| `bash_violation` | `error` | `low` |

## Time-Windowing

Prevents duplicate escalations for the same session within a configurable window (default 15 minutes):

```
Session A: Escalation at T0
Session A: Escalation at T0+5min  ← Skipped (within window)
Session A: Escalation at T0+20min ← Published (after window)
Session B: Escalation at T0+10min ← Published (different session)
```

This prevents spam when a session repeatedly encounters the same issue.

## Testing

Run tests with coverage:

```bash
cd internal/astrocyte
go test -v -cover
```

### Test Coverage

- ✓ Escalation tracker (concurrent sessions, time-windowing)
- ✓ Incident processing (symptom mapping, event creation)
- ✓ File watching (polling, JSONL parsing)
- ✓ Time-windowing (duplicate prevention)
- ✓ Event validation (schema compliance)
- ✓ Multiple concurrent sessions

Coverage target: ≥80%

## Configuration

### Environment Variables

- `AGM_EVENTBUS_PORT`: WebSocket server port (default: 8080)
- `AGM_EVENTBUS_MAX_CLIENTS`: Maximum concurrent clients (default: 100)

### Watcher Configuration

```go
// Custom incidents file
watcher := astrocyte.NewWatcher(hub, "/custom/path/incidents.jsonl", 15*time.Minute)

// Custom escalation window (5 minutes)
watcher := astrocyte.NewWatcher(hub, "", 5*time.Minute)

// Default settings
watcher := astrocyte.NewWatcher(hub, "", 0)  // Uses ~/.agm/astrocyte/incidents.jsonl, 15min window
```

## Troubleshooting

### No events published

1. Check Astrocyte is running and logging incidents:
   ```bash
   ls -la ~/.agm/astrocyte/incidents.jsonl
   tail -f ~/.agm/astrocyte/incidents.jsonl
   ```

2. Verify watcher started successfully:
   ```
   ✓ Astrocyte watcher started (file: ~/.agm/astrocyte/incidents.jsonl, window: 15m0s)
   ```

3. Check for processing logs:
   ```
   ✓ Processed 1 new incident(s)
   📡 Published escalation event: session=my-session, type=error, symptom=stuck_mustering
   ```

### Duplicate escalations

Reduce the escalation window:

```go
watcher := astrocyte.NewWatcher(hub, "", 5*time.Minute)  // 5-minute window
```

### Performance issues

Increase the poll interval (modify `pollInterval` constant in `watcher.go`):

```go
const pollInterval = 10 * time.Second  // Default: 5 seconds
```

## Performance

- **File polling**: 5-second intervals (configurable)
- **Memory**: ~1KB per tracked session
- **CPU**: Minimal (only processes new lines)
- **Thread-safety**: All operations are concurrent-safe

## Agent Compatibility

Astrocyte's monitoring capabilities vary by agent type due to architectural differences:

### Tmux-Based Agents (Claude, Gemini) ✅ Full Support

Astrocyte provides comprehensive monitoring for tmux-based agents:

**Detectors**:
- ✅ Bash pattern violations (cat, pipe, cd usage)
- ✅ Stuck state detection (mustering, waiting, cursor frozen)
- ✅ Permission prompt detection
- ✅ Beads protection (guards against .beads/ modifications)
- ✅ Git state monitoring

**Recovery**:
- ✅ Auto-recovery via ESC key injection
- ✅ Recovery messages via `agm session send`
- ✅ Incident logging to `~/.agm/astrocyte/incidents.jsonl`
- ✅ EventBus escalation events

### API-Based Agents (OpenAI) ⚠️ Limited Support

OpenAI sessions are API-only with no tmux pane to monitor:

**Detectors**:
- ❌ Bash pattern detection (no pane snapshot)
- ❌ Stuck state detection (no cursor position)
- ❌ Permission prompt detection (API responses only)
- ❌ Real-time monitoring (no tmux pane)

**Recovery**:
- ✅ Manual recovery messages via `agm session send`
- ✅ Incident logging (manual incidents can be created)
- ⚠️ Auto-recovery not applicable (no pane to inject keys)
- ✅ EventBus escalation events (if incidents manually created)

**Manual Incident Creation**:

You can manually create incidents for OpenAI sessions:

```bash
# Create manual incident for OpenAI session
cat >> ~/.agm/astrocyte/incidents.jsonl << EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "session_id": "openai-session-uuid",
  "session_name": "my-openai-session",
  "symptom": "api_timeout",
  "duration_minutes": 5,
  "detection_heuristic": "manual",
  "pane_snapshot": "API request timeout",
  "recovery_attempted": false,
  "diagnosis_filed": false,
  "cascade_depth": 0,
  "circuit_breaker_triggered": false
}
EOF
```

**Sending Recovery Messages**:

```bash
# Send recovery message to OpenAI session
agm session send my-openai-session --sender astrocyte \
  --prompt "⚠️ Your session may be experiencing API delays. Please check the response."
```

### Future Enhancement: API-Based Monitoring

Astrocyte could be extended to monitor API-based sessions via:

1. **Conversation History Analysis**:
   - Parse message history for bash violations
   - Detect tool use patterns
   - Monitor for unsafe commands in code blocks

2. **API Latency Monitoring**:
   - Track API response times
   - Detect timeout patterns
   - Alert on rate limit errors

3. **Token Usage Monitoring**:
   - Track context window utilization
   - Detect zero-token galloping patterns
   - Alert on context overflow

This enhancement is **out of scope** for the current implementation but documented for future consideration.

---

## Future Enhancements

1. **File watching** using `fsnotify` instead of polling
2. **Configurable severity mapping** (YAML config)
3. **Event filtering** (only publish certain symptom types)
4. **Metrics** (Prometheus-style counters for escalations)
5. **Event persistence** (SQLite log of published events)
6. **API-based monitoring** (conversation history analysis, latency tracking)
