# AGM Daemon - Architecture

## System Overview

The AGM Daemon is a background message delivery service that implements state-aware routing for inter-session communication. It polls a persistent SQLite message queue every 30 seconds, detects the current state of target Claude sessions (READY, THINKING, COMPACTING, OFFLINE), and delivers queued messages only when sessions are ready to receive them. The daemon ensures reliable message delivery through retry logic, acknowledgment tracking, and exponential backoff for failed deliveries.

## Architecture Diagram

```
┌───────────────────────────────────────────────────────────────┐
│                    Message Producers                           │
│                                                                │
│  ┌──────────────────┐  ┌──────────────────┐  ┌────────────┐  │
│  │ Claude Session A │  │ Claude Session B │  │ Coordinator│  │
│  │ (agm send)       │  │ (agm send)       │  │ (route)    │  │
│  └────────┬─────────┘  └────────┬─────────┘  └──────┬─────┘  │
└───────────┼──────────────────────┼────────────────────┼────────┘
            │                      │                    │
            │ Enqueue              │ Enqueue            │ Enqueue
            ▼                      ▼                    ▼
┌───────────────────────────────────────────────────────────────┐
│                    Message Queue (SQLite)                      │
│  Location: ~/.agm/queue.db                                    │
│  Mode: WAL (Write-Ahead Logging)                              │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ message_queue table                                       │ │
│  │  - message_id (PK)                                        │ │
│  │  - from_session                                           │ │
│  │  - to_session                                             │ │
│  │  - message (content)                                      │ │
│  │  - status (pending/delivered/failed)                      │ │
│  │  - attempt_count (0-3)                                    │ │
│  │  - ack_required, ack_received, ack_timeout                │ │
│  └──────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────┘
            ▲                      │
            │ GetAllPending()      │ MarkDelivered()
            │                      │ IncrementAttempt()
            │                      ▼
┌───────────────────────────────────────────────────────────────┐
│                     AGM Daemon Process                         │
│  Binary: agm-daemon                                           │
│  PID File: ~/.agm/daemon.pid                                  │
│  Logs: ~/.agm/logs/daemon/daemon.log                          │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ cmd/agm-daemon/main.go - Entry Point                      │ │
│  │  - Initialize logger (file-based)                         │ │
│  │  - Open message queue (SQLite connection)                 │ │
│  │  - Create AckManager                                      │ │
│  │  - Create daemon with Config                              │ │
│  │  - Start daemon (blocks until shutdown)                   │ │
│  └────────────────────────┬─────────────────────────────────┘ │
│                           │                                   │
│  ┌────────────────────────▼─────────────────────────────────┐ │
│  │ internal/daemon/daemon.go - Core Daemon Logic             │ │
│  │                                                            │ │
│  │  ┌───────────────────────────────────────────────────┐   │ │
│  │  │ Start() - Main Event Loop                         │   │ │
│  │  │  - Write PID file (prevent multiple instances)    │   │ │
│  │  │  - Setup signal handling (SIGTERM, SIGINT)        │   │ │
│  │  │  - Create ticker (30s poll interval)              │   │ │
│  │  │  - Loop:                                           │   │ │
│  │  │    select {                                        │   │ │
│  │  │      case <-ticker.C:                              │   │ │
│  │  │        deliverPending()                            │   │ │
│  │  │        ackManager.CheckTimeout()                   │   │ │
│  │  │      case sig := <-sigCh:                          │   │ │
│  │  │        Stop()                                      │   │ │
│  │  │    }                                               │   │ │
│  │  └───────────────────────────────────────────────────┘   │ │
│  │                                                            │ │
│  │  ┌───────────────────────────────────────────────────┐   │ │
│  │  │ deliverPending() - Delivery Loop                  │   │ │
│  │  │  1. queue.GetAllPending() → [QueueEntry]          │   │ │
│  │  │  2. For each entry:                                │   │ │
│  │  │     - session.ResolveIdentifier(entry.To)          │   │ │
│  │  │     - session.DetectState(sessionName)             │   │ │
│  │  │     - Route by state:                              │   │ │
│  │  │       • READY → deliverMessage()                   │   │ │
│  │  │       • THINKING/COMPACTING → defer (skip)             │   │ │
│  │  │       • OFFLINE → retryLater()                     │   │ │
│  │  └───────────────────────────────────────────────────┘   │ │
│  │                                                            │ │
│  │  ┌───────────────────────────────────────────────────┐   │ │
│  │  │ deliverMessage() - Actual Delivery                │   │ │
│  │  │  1. tmux.SendMultiLinePromptSafe(sessionName, msg)│   │ │
│  │  │  2. session.UpdateSessionState(path, THINKING, src)    │   │ │
│  │  │  3. queue.MarkDelivered(messageID)                 │   │ │
│  │  │  4. ackManager.SendAck(messageID)                  │   │ │
│  │  └───────────────────────────────────────────────────┘   │ │
│  │                                                            │ │
│  │  ┌───────────────────────────────────────────────────┐   │ │
│  │  │ retryLater() - Retry Logic                        │   │ │
│  │  │  if attemptCount >= MaxRetries:                    │   │ │
│  │  │    queue.MarkPermanentlyFailed(messageID)          │   │ │
│  │  │  else:                                             │   │ │
│  │  │    queue.IncrementAttempt(messageID)               │   │ │
│  │  │    backoff = 5s * 2^attemptCount                   │   │ │
│  │  └───────────────────────────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────┘
            │                      │                      │
            │ ResolveIdentifier   │ DetectState          │ SendMessage
            ▼                      ▼                      ▼
┌───────────────────────────────────────────────────────────────┐
│                    Internal Libraries                          │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ internal/session - Session Management                     │ │
│  │  - ResolveIdentifier(name) → (manifest, manifestPath)     │ │
│  │  - DetectState(sessionName) → StateType                   │ │
│  │  - UpdateSessionState(path, state, updatedBy)             │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ internal/messages - Queue & Acknowledgments               │ │
│  │  - MessageQueue (SQLite wrapper)                          │ │
│  │    • GetAllPending() → []*QueueEntry                      │ │
│  │    • MarkDelivered(messageID)                             │ │
│  │    • IncrementAttempt(messageID)                          │ │
│  │    • MarkPermanentlyFailed(messageID)                     │ │
│  │  - AckManager                                             │ │
│  │    • WaitForAck(messageID, timeout)                       │ │
│  │    • SendAck(messageID)                                   │ │
│  │    • CheckTimeout() → timedOutCount                       │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ internal/tmux - Tmux Integration                          │ │
│  │  - SendMultiLinePromptSafe(sessionName, message)          │ │
│  │  - SessionExists(sessionName) → bool                      │ │
│  └──────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────┘
            │                      │
            │ Read Manifest        │ Send Keys
            ▼                      ▼
┌───────────────────────────────────────────────────────────────┐
│                   File System & Tmux Layer                     │
│                                                                │
│  Session Manifests:                                           │
│  ~/.agm/sessions/{session-name}/manifest.json                 │
│  ├── version: 3                                               │
│  ├── state: "READY" | "THINKING" | "COMPACTING" | "OFFLINE"      │
│  ├── state_updated_at: timestamp                              │
│  ├── state_updated_by: "daemon" | "compact-hook" | ...       │
│  └── tmux.session_name: "tmux-session-1"                      │
│                                                                │
│  Tmux Sessions:                                               │
│  tmux list-sessions:                                          │
│  ├── tmux-session-1 (active)   ← Claude running              │
│  ├── tmux-session-2 (active)   ← Claude idle                 │
│  └── tmux-session-3 (detached) ← Claude compacting           │
│                                                                │
│  Daemon Files:                                                │
│  ~/.agm/daemon.pid              ← Process ID (lock file)      │
│  ~/.agm/logs/daemon/daemon.log  ← Structured logs            │
│  ~/.agm/queue.db                ← SQLite database (WAL mode)  │
└───────────────────────────────────────────────────────────────┘
```

## Component Details

### 1. Main Entry Point (cmd/agm-daemon/main.go)

**Responsibilities:**
- Initialize base directories (`~/.agm`, `~/.agm/logs/daemon`)
- Create file-based logger (daemon.log)
- Open message queue (SQLite connection)
- Create AckManager for message acknowledgments
- Create daemon with Config struct
- Start daemon (blocks until shutdown signal)

**Key Code:**
```go
func main() {
    homeDir, _ := os.UserHomeDir()
    baseDir := filepath.Join(homeDir, ".agm")
    logDir := filepath.Join(baseDir, "logs", "daemon")
    pidFile := filepath.Join(baseDir, "daemon.pid")

    logger := log.New(logFile, "[daemon] ", log.LstdFlags)
    queue, _ := messages.NewMessageQueue()
    ackManager := messages.NewAckManager(queue)

    cfg := daemon.Config{
        BaseDir:    baseDir,
        LogDir:     logDir,
        PIDFile:    pidFile,
        Queue:      queue,
        AckManager: ackManager,
        Logger:     logger,
    }

    d := daemon.NewDaemon(cfg)
    d.Start()  // Blocks until SIGTERM/SIGINT
}
```

### 2. Daemon Core (internal/daemon/daemon.go)

**Responsibilities:**
- Manage daemon lifecycle (start, stop, PID file)
- Poll message queue every 30 seconds
- Detect session states via manifest + tmux checks
- Route messages based on session state
- Implement retry logic with exponential backoff
- Handle acknowledgment timeouts

**Key Structures:**
```go
type Daemon struct {
    cfg    Config
    ticker *time.Ticker
    ctx    context.Context
    cancel context.CancelFunc
}

type Config struct {
    BaseDir    string
    LogDir     string
    PIDFile    string
    Queue      *messages.MessageQueue
    AckManager *messages.AckManager
    Logger     *log.Logger
}
```

**Main Event Loop:**
```go
func (d *Daemon) Start() error {
    d.writePIDFile()
    defer d.removePIDFile()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    d.ticker = time.NewTicker(PollInterval)  // 30s
    defer d.ticker.Stop()

    for {
        select {
        case <-d.ctx.Done():
            return nil
        case sig := <-sigCh:
            d.Stop()
            return nil
        case <-d.ticker.C:
            d.deliverPending()
            d.cfg.AckManager.CheckTimeout()
        }
    }
}
```

### 3. Message Queue (internal/messages/queue.go)

**Responsibilities:**
- Persistent message storage in SQLite
- FIFO ordering with priority support
- Status tracking (pending, delivered, failed)
- Atomic enqueue/dequeue operations
- WAL mode for concurrent access

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
    delivered_at TIMESTAMP,
    ack_required INTEGER NOT NULL DEFAULT 1,
    ack_received INTEGER NOT NULL DEFAULT 0,
    ack_timeout TIMESTAMP
);

CREATE INDEX idx_pending ON message_queue(to_session, status, created_at)
    WHERE status = 'pending';
```

**Key Methods:**
```go
func (q *MessageQueue) GetAllPending() ([]*QueueEntry, error)
func (q *MessageQueue) MarkDelivered(messageID string) error
func (q *MessageQueue) IncrementAttempt(messageID string) error
func (q *MessageQueue) MarkPermanentlyFailed(messageID string) error
```

### 4. Acknowledgment Manager (internal/messages/ack.go)

**Responsibilities:**
- Track pending acknowledgments
- Block until ack received or timeout (60s)
- Detect timed-out acknowledgments
- Requeue messages on timeout

**Key Structure:**
```go
type AckManager struct {
    queue       *MessageQueue
    pendingAcks map[string]chan error  // messageID → ack channel
    mu          sync.RWMutex
}
```

**Acknowledgment Flow:**
```go
// Sender side (daemon):
err := ackManager.SendAck(messageID)
// Sets ack_received=1, ack_timeout=NULL

// Recipient side (session):
err := ackManager.WaitForAck(messageID, 60*time.Second)
// Blocks on channel until SendAck() called or timeout
```

### 5. Session State Detection (internal/session/state.go)

**Responsibilities:**
- Load session manifests from filesystem
- Detect current session state (READY, THINKING, COMPACTING, OFFLINE)
- Check tmux session existence
- Update manifest state field

**State Detection Logic:**
```go
func DetectState(sessionName string) (StateType, error) {
    // 1. Load manifest
    manifest, _, err := ResolveIdentifier(sessionName, "")
    if err != nil {
        return StateOffline, nil  // Session doesn't exist
    }

    // 2. Check tmux session exists
    if !tmux.SessionExists(manifest.Tmux.SessionName) {
        return StateOffline, nil
    }

    // 3. Return manifest state (set by hooks)
    switch manifest.State {
    case StateReady:
        return StateReady, nil
    case StateBusy:
        return StateBusy, nil
    case StateCompacting:
        return StateCompacting, nil
    default:
        return StateBusy, nil  // Safe default
    }
}
```

## State Machine

### Session State Transitions

```
State Transitions (managed by hooks):
  READY ──[message-received]──> THINKING
  THINKING ──[message-complete]──> READY
  READY ──[compact-start]──> COMPACTING
  COMPACTING ──[compact-complete]──> READY
  (any) ──[session-exit]──> OFFLINE
  OFFLINE ──[session-start]──> READY
```

### Message Delivery State Machine

```
Message States:
  PENDING ──[session READY]──> send message
                                   │
                                   ├──> SUCCESS ──> DELIVERED ──> ACK_RECEIVED
                                   │
                                   └──> FAIL ──> increment attempts
                                                      │
                                                      ├──> attempts < 3 ──> PENDING (retry)
                                                      │
                                                      └──> attempts >= 3 ──> PERMANENTLY_FAILED

  PENDING ──[session THINKING/COMPACTING]──> defer (stay PENDING)
  PENDING ──[session OFFLINE]──> increment attempts (retry logic)
```

## Concurrency Model

### Goroutines

1. **Main Goroutine**: Event loop (signal handling, ticker)
2. **No Background Goroutines**: All work done synchronously in main loop

**Why Synchronous?**
- Polling interval is 30s (no need for parallelism)
- SQLite WAL mode allows concurrent reads but single writer
- Simplifies error handling and logging
- Avoids race conditions

### Thread Safety

- **MessageQueue**: Thread-safe via SQLite WAL mode
- **AckManager**: Protected by `sync.RWMutex`
- **No Shared State**: Daemon processes messages sequentially

## Performance Characteristics

### Latency

| Operation | Typical Latency | Notes |
|-----------|-----------------|-------|
| Queue poll | 50-100ms | SQLite query with index |
| State detection | 20-50ms | Manifest read + tmux check |
| Message delivery | 100-200ms | tmux send-keys |
| Full delivery cycle | 300-500ms | End-to-end for single message |

### Throughput

- **Max messages/poll**: 100 messages
- **Poll interval**: 30s
- **Theoretical max throughput**: ~200 messages/minute
- **Practical throughput**: 50-100 messages/minute (accounting for retries)

### Resource Usage

- **Memory**: ~10MB baseline + ~1KB per queued message
- **CPU**: <1% (mostly I/O wait on SQLite/tmux)
- **Disk I/O**: ~10KB/s (SQLite writes + log appends)

## Error Handling Strategy

### Error Categories

1. **Fatal Errors** (Exit daemon):
   - PID file already exists (daemon running)
   - Cannot create log directory
   - Cannot open message queue database

2. **Retriable Errors** (Retry on next poll):
   - Queue.GetAllPending() fails (database locked)
   - Session manifest not found (session might be starting)
   - tmux send-keys fails (session might be busy)

3. **Permanent Failures** (Move to DLQ):
   - 3 delivery attempts exceeded
   - Message marked as permanently failed

### Error Recovery

- **Transient Errors**: Log warning, continue to next poll cycle
- **Permanent Errors**: Mark message as failed, move to dead letter queue
- **Database Corruption**: Daemon exits (manual intervention required)

## Security & Privacy

### Security Principles

1. **Local Only**: Daemon runs locally, no network exposure
2. **PID File Lock**: Prevents multiple daemon instances (race condition prevention)
3. **File Permissions**: Queue database (0600), logs (0644), manifests (0644)
4. **No Credentials**: No API keys or secrets in queue or logs

### Privacy Guarantees

- **Message Content**: Logged with truncation (max 60 chars)
- **Full Content**: Only in SQLite database (not in logs)
- **State Information**: Only state enum (READY/THINKING/etc), not conversation content

## Deployment Considerations

### Single Instance Enforcement

- PID file prevents multiple daemon instances
- Check: `daemon.IsRunning(pidFile)` before starting
- Stale PID file cleanup on start

### Daemon Lifecycle

```bash
# Start daemon
agm daemon start
# → Creates PID file, forks background process, returns immediately

# Check status
agm daemon status
# → Reads PID file, checks if process exists, queries queue state

# Stop daemon
agm daemon stop
# → Sends SIGTERM to PID, waits for graceful shutdown, removes PID file

# Restart daemon
agm daemon restart
# → Equivalent to stop + start
```

### Log Rotation

- **Current**: Daemon appends to `daemon.log` indefinitely
- **Recommendation**: Use logrotate or manual log cleanup
- **Future**: Built-in log rotation (size-based or time-based)

## Testing Strategy

### Unit Tests

- Daemon lifecycle (Start, Stop, PID file management)
- State detection logic (all state enum values)
- Retry logic (attempt count, exponential backoff)
- Truncate message helper (log safety)

### Integration Tests

- End-to-end message delivery to real tmux session
- State transition handling (READY → THINKING → READY)
- Acknowledgment protocol (timeout, requeue)
- Queue persistence across daemon restarts

### Performance Tests

- Queue poll performance (100 messages)
- Delivery latency (enqueue to delivery)
- Resource usage (memory, CPU) under load

## Future Enhancements

### V2 Features

1. **Priority Queue**: High-priority messages delivered first
2. **Rate Limiting**: Max messages per session per minute
3. **Dead Letter Queue UI**: Web interface to inspect failed messages
4. **Metrics Export**: Prometheus-compatible metrics endpoint
5. **Configurable Poll Interval**: User-adjustable via config file
6. **Delivery Webhooks**: Notify external systems on delivery events

### V3 Features

1. **Multi-Daemon Support**: Horizontal scaling with leader election
2. **Remote Queue**: Support for networked message queue (Redis, RabbitMQ)
3. **End-to-End Encryption**: Encrypt message content at rest

## Related Documentation

### Architecture Decision Records

- **[ADR-006: Message Queue Architecture](../../docs/adr/ADR-006-message-queue-architecture.md)**: Queue design rationale
- **[ADR-007: Hook-Based State Detection](../../docs/adr/ADR-007-hook-based-state-detection.md)**: State detection approach
- **[ADR-008: Status Aggregation Pattern](../../docs/adr/ADR-008-status-aggregation.md)**: Status query design

### Specifications

- **[SPEC.md](./SPEC.md)**: Daemon specification (features, API, requirements)
- **[Message Queue SPEC](../../internal/messages/SPEC.md)**: Queue implementation details

### Implementation

- **[daemon.go](../../internal/daemon/daemon.go)**: Core daemon implementation
- **[queue.go](../../internal/messages/queue.go)**: Message queue implementation
- **[ack.go](../../internal/messages/ack.go)**: Acknowledgment protocol

---

**Version**: 1.0
**Last Updated**: 2026-02-19
**Implementation**: Phase 2, Tasks 2.1, 2.3
**Beads**: oss-clnn (Delivery Daemon), oss-ylk4 (Acknowledgment)
