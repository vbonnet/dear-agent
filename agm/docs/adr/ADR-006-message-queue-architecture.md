# ADR-006: Message Queue Architecture

## Status
**Accepted** (2026-02-01)

## Context

AGM needs reliable message delivery between Claude sessions for multi-session coordination. Sessions may be in different states (DONE, WORKING, COMPACTING, OFFLINE) and messages must be queued when recipients are unavailable.

**Requirements:**
- Persistent message storage (survives process restart)
- FIFO delivery ordering per recipient
- Retry logic for failed deliveries
- Duplicate message detection
- Delivery status tracking (pending, delivered, failed)
- Thread-safe concurrent access
- Minimal dependencies

**Constraints:**
- Must work on all platforms (Linux, macOS, Windows)
- No external services (Redis, RabbitMQ) - keep deployment simple
- Must integrate with existing tmux-based session management

## Decision

**Implement SQLite-based message queue with WAL mode.**

### Architecture

```go
type MessageQueue struct {
    db *sql.DB
}

type QueueEntry struct {
    MessageID    string
    From         string
    To           string
    Message      string
    Priority     int
    Status       string  // "pending", "delivered", "failed"
    AttemptCount int
    CreatedAt    time.Time
    DeliveredAt  *time.Time
}
```

**Schema:**
```sql
CREATE TABLE message_queue (
    message_id TEXT PRIMARY KEY,
    from_session TEXT NOT NULL,
    to_session TEXT NOT NULL,
    message TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    delivered_at TIMESTAMP
);

CREATE INDEX idx_pending ON message_queue(to_session, status, created_at)
    WHERE status = 'pending';
```

**WAL Mode:**
```go
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
```

## Alternatives Considered

### 1. In-Memory Queue + JSON Persistence
**Pros:** Simple, fast reads
**Cons:** Risk of data loss on crash, complex recovery logic, no ACID guarantees
**Rejected:** Reliability is critical for coordination messages

### 2. Redis-Based Queue
**Pros:** Battle-tested, rich features (pub/sub, TTL, sorted sets)
**Cons:** External dependency, deployment complexity, platform-specific
**Rejected:** Violates "minimal dependencies" constraint

### 3. File-Based Queue (One File Per Message)
**Pros:** Very simple, no database
**Cons:** Poor performance at scale, no atomic operations, complex locking
**Rejected:** Doesn't scale beyond 100 messages

### 4. Embedded Database (BoltDB, BadgerDB)
**Pros:** Pure Go, high performance
**Cons:** Additional dependency, less mature than SQLite, platform-specific bugs
**Rejected:** SQLite is more battle-tested and has better tooling

## Consequences

### Positive
✅ **Reliable:** ACID transactions ensure no message loss
✅ **Portable:** SQLite works on all platforms without external services
✅ **Debuggable:** Can use `sqlite3` CLI to inspect queue state
✅ **Scalable:** Handles 10,000+ messages efficiently with indexes
✅ **Thread-safe:** WAL mode enables concurrent readers + single writer
✅ **Simple deployment:** Single file database, no setup required

### Negative
❌ **Write bottleneck:** Single writer limits throughput (acceptable for AGM use case)
❌ **No pub/sub:** Must poll for new messages (daemon polls every 30s)
❌ **File locking:** Can cause issues on network filesystems (documented limitation)

### Neutral
🔵 **Database file grows:** Requires periodic cleanup of delivered messages
🔵 **No built-in TTL:** Must implement message expiration manually

## Implementation

### Key Files
- `internal/messages/queue.go` (340 lines)
- `internal/messages/queue_test.go` (580 lines)
- Database path: `~/.agm/queue.db`

### Critical Methods
```go
func NewMessageQueue() (*MessageQueue, error)
func (q *MessageQueue) Enqueue(from, to, message string, priority int) (string, error)
func (q *MessageQueue) GetAllPending() ([]*QueueEntry, error)
func (q *MessageQueue) MarkDelivered(messageID string) error
func (q *MessageQueue) MarkPermanentlyFailed(messageID string) error
func (q *MessageQueue) IncrementAttempt(messageID string) error
```

### WAL Mode Configuration
```go
_, err := db.Exec("PRAGMA journal_mode=WAL")
// WAL mode benefits:
// - Concurrent readers don't block writers
// - Better crash recovery
// - Faster writes (group commit)
```

### Deduplication Strategy
- Primary key on `message_id` (UUID v4)
- Enqueue returns existing ID if duplicate detected
- No retry storms from duplicate sends

## Validation

### Functional Tests
- ✅ Enqueue and dequeue FIFO ordering
- ✅ Priority ordering (high priority first)
- ✅ Retry limit enforcement (max 3 attempts)
- ✅ Concurrent enqueue/dequeue safety
- ✅ Database file corruption recovery

### Performance Tests
- ✅ 1,000 messages enqueued in <100ms
- ✅ 10,000 messages queried in <50ms
- ✅ Concurrent access from 10 goroutines (no deadlocks)

### Integration Tests
- ✅ Daemon polls queue every 30s
- ✅ Messages delivered when session becomes DONE
- ✅ Failed messages retried with exponential backoff

## Related Decisions
- **ADR-007:** Hook-Based State Detection (uses queue for deferred delivery)
- **ADR-008:** Status Aggregation (queries queue for pending message counts)
- **Phase 2 Task 2.3:** Message Acknowledgment (extends queue with ack tracking)

## Migration Notes

**Backward Compatibility:**
- Schema is additive-only (new columns added with ALTER TABLE)
- No breaking changes to existing code

**Future Enhancements:**
- Dead letter queue for permanently failed messages
- Message TTL/expiration
- Queue metrics (throughput, latency, backlog size)

---

**Deciders:** Foundation Engineering
**Date:** 2026-02-01
**Implementation:** Phase 1, Task 1.1
**Bead:** oss-c6mc (closed)
