# ADR-008: Status Aggregation Pattern

## Status
**Accepted** (2026-02-02)

## Context

AGM manages multiple Claude sessions that can be in different states (DONE, WORKING, COMPACTING, OFFLINE). Users need visibility into the health and status of the entire session fleet to answer questions like:

- "Which sessions are available to receive messages?"
- "Why are messages not being delivered?"
- "What is the current queue backlog?"
- "Are there any failed deliveries?"

**Requirements:**
- Aggregate status from multiple sessions efficiently
- Display queue backlog per session
- Show delivery failures and retry counts
- Provide actionable troubleshooting information
- Support both CLI output and programmatic access

**Constraints:**
- Must not introduce performance overhead (no global locks)
- Should work with existing manifest + queue infrastructure
- CLI output must be human-readable and parseable

## Decision

**Implement status aggregation as a read-only query pattern that combines session manifests + queue state.**

### Architecture

```go
type SessionStatus struct {
    SessionName     string
    State           StateType    // DONE, WORKING, COMPACTING, OFFLINE
    StateUpdatedAt  time.Time
    QueuedMessages  int          // Pending messages to this session
    FailedMessages  int          // Permanently failed messages
    LastActivity    time.Time    // Most recent state change or message
}

func AggregateStatus() ([]SessionStatus, error) {
    // 1. List all sessions (from manifest directory)
    sessions := session.ListAll()

    // 2. For each session, query state + queue
    statuses := []SessionStatus{}
    for _, s := range sessions {
        state := session.DetectState(s.Name)
        queued := queue.CountPending(s.Name)
        failed := queue.CountFailed(s.Name)

        statuses = append(statuses, SessionStatus{
            SessionName:    s.Name,
            State:          state,
            StateUpdatedAt: s.StateUpdatedAt,
            QueuedMessages: queued,
            FailedMessages: failed,
            LastActivity:   max(s.StateUpdatedAt, s.LastMessageAt),
        })
    }

    return statuses, nil
}
```

### CLI Output Format

```bash
$ agm daemon status

AGM Daemon Status
═════════════════════════════════════════════════════════════════

Session Status:
┌────────────────────┬───────────┬──────────┬────────┬──────────┐
│ Session            │ State     │ Queued   │ Failed │ Updated  │
├────────────────────┼───────────┼──────────┼────────┼──────────┤
│ work-session-1     │ DONE      │ 0        │ 0      │ 2m ago   │
│ research-session-2 │ WORKING   │ 3        │ 0      │ 30s ago  │
│ archived-session-3 │ OFFLINE   │ 0        │ 2      │ 5d ago   │
└────────────────────┴───────────┴──────────┴────────┴──────────┘

Queue Summary:
  Pending: 3 messages
  Failed:  2 messages
  Total:   5 messages

Daemon: Running (PID 12345)
Poll Interval: 30s
Last Poll: 5s ago
```

## Alternatives Considered

### 1. Push-Based Status Updates (Event Stream)
**Implementation:** Sessions push status changes to central event log
**Pros:** Real-time updates, no polling
**Cons:** Complex event handling, requires background daemon per session, higher overhead
**Rejected:** Over-engineered for status aggregation use case

### 2. Centralized Status Database
**Implementation:** All sessions write status to shared SQLite database
**Pros:** Single source of truth, fast queries
**Cons:** Write contention, requires locking, adds complexity
**Rejected:** Manifests already contain state, don't duplicate

### 3. REST API + Webhook Pattern
**Implementation:** Daemon exposes REST API, sessions POST status updates
**Pros:** Industry standard, machine-readable
**Cons:** Requires HTTP server, port management, auth complexity
**Rejected:** CLI-first design, no need for HTTP

### 4. File-Based Status Snapshots
**Implementation:** Each session writes `~/.agm/status-{session}.json`
**Pros:** Simple, no database
**Cons:** Stale files, manual cleanup, no atomic reads
**Rejected:** Manifests already contain state, don't duplicate

## Consequences

### Positive
✅ **Simple:** No new infrastructure, queries existing manifest + queue
✅ **Efficient:** Read-only queries, no locks, parallelizable
✅ **Accurate:** Always reflects current state (no caching staleness)
✅ **Actionable:** Shows queue backlog + failed messages for troubleshooting
✅ **Extensible:** Easy to add new fields (e.g., session uptime, message throughput)

### Negative
❌ **Query overhead:** Must scan all manifests + query queue (acceptable for <100 sessions)
❌ **No real-time updates:** Status is point-in-time snapshot (user must re-run command)

### Neutral
🔵 **Performance:** O(N) where N = number of sessions (fast enough for typical use)
🔵 **Caching:** Could add caching if needed (not implemented in v1)

## Implementation

### Key Files
- `cmd/agm/daemon_cmd.go` (195 lines) - CLI commands
- `internal/daemon/daemon.go` (367 lines) - Status aggregation logic
- `internal/messages/queue.go` (enhanced with CountPending, CountFailed)

### Status Query Methods
```go
// Count pending messages for a session
func (q *MessageQueue) CountPending(toSession string) (int, error) {
    var count int
    err := q.db.QueryRow(`
        SELECT COUNT(*) FROM message_queue
        WHERE to_session = ? AND status = 'pending'
    `, toSession).Scan(&count)
    return count, err
}

// Count failed messages for a session
func (q *MessageQueue) CountFailed(toSession string) (int, error) {
    var count int
    err := q.db.QueryRow(`
        SELECT COUNT(*) FROM message_queue
        WHERE to_session = ? AND status = 'failed'
    `, toSession).Scan(&count)
    return count, err
}
```

### CLI Implementation
```go
func statusCmd() error {
    // Check if daemon is running
    running := daemon.IsRunning(pidFile)

    // Aggregate session status
    statuses, err := aggregateStatus()
    if err != nil {
        return err
    }

    // Display table
    printStatusTable(statuses)
    printQueueSummary(statuses)
    printDaemonStatus(running)

    return nil
}
```

## Validation

### Functional Tests
- ✅ Status aggregation with 0 sessions (empty output)
- ✅ Status aggregation with 10 sessions (performance <100ms)
- ✅ Failed message counts accurate
- ✅ Queue backlog counts accurate per session

### Integration Tests
- ✅ CLI output matches expected format
- ✅ Status reflects recent state changes
- ✅ Works with daemon running and stopped

### Performance Tests
- ✅ 100 sessions aggregated in <500ms
- ✅ Concurrent status queries don't block queue operations

## Related Decisions
- **ADR-006:** Message Queue Architecture (status queries queue state)
- **ADR-007:** Hook-Based State Detection (status queries session state)
- **Phase 2 Task 2.2:** Coordinator Pattern (uses status for load balancing)

## Migration Notes

**Backward Compatibility:**
- Status aggregation is read-only (no schema changes)
- Works with existing manifests and queue

**Future Enhancements:**
- Persistent status snapshots for historical analysis
- Alerting on abnormal patterns (e.g., 10+ failed messages)
- Dashboard UI (web-based status viewer)
- Metrics export (Prometheus, StatsD)

---

**Deciders:** Foundation Engineering
**Date:** 2026-02-02
**Implementation:** Phase 1, Task 1.3
**Bead:** oss-l7xf (closed)
