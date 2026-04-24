# AGM Daemon - Specification

## Overview

The AGM Daemon is a background service that delivers queued messages to Claude sessions based on their current state. It polls the message queue every 30 seconds, detects session state transitions, and delivers messages only when the target session is in the READY state.

## Objectives

1. **Reliable Message Delivery**: Ensure messages reach Claude sessions when they are ready to process them
2. **State-Aware Routing**: Deliver messages only when sessions are READY, defer when THINKING/COMPACTING, retry when OFFLINE
3. **Message Persistence**: Queue messages in SQLite database with retry logic and delivery tracking
4. **Message Acknowledgment**: Track message delivery with acknowledgments and timeout detection
5. **Operational Visibility**: Provide status commands to view queue state and daemon health

## Use Cases

### Primary Use Cases

1. **Asynchronous Message Delivery**
   - Session A sends message to Session B
   - Message queued in persistent SQLite database
   - Daemon delivers when Session B becomes READY
   - Acknowledgment confirms delivery

2. **Multi-Session Coordination**
   - Coordinator routes messages to multiple sessions
   - Load balancing distributes work across available sessions
   - Conflict resolution ensures work isn't duplicated
   - Status aggregation shows fleet health

3. **Deferred Delivery During Busy States**
   - Session is THINKING (processing previous message)
   - Daemon defers delivery, leaves message in queue
   - Session transitions to READY
   - Daemon delivers on next poll cycle (within 30s)

### Secondary Use Cases

1. **Retry Logic for Offline Sessions**
   - Session is OFFLINE (not running)
   - Daemon increments attempt count, implements exponential backoff
   - After 3 failed attempts, marks message as permanently failed
   - Dead letter queue (DLQ) available for inspection

2. **Acknowledgment Timeout Detection**
   - Message delivered but acknowledgment not received within 60s
   - Daemon detects timeout, requeues message
   - Prevents lost messages due to session crashes

3. **Status Visibility**
   - CLI command shows daemon running status
   - Displays pending messages per session
   - Shows failed message counts for troubleshooting
   - Last poll time and next poll ETA

## Architecture

### Core Components

```
┌──────────────────────────────────────────────────────────────┐
│                         AGM Daemon                            │
│                                                               │
│  ┌────────────────┐      ┌──────────────┐                    │
│  │  Poll Timer    │──────>│ Delivery     │                    │
│  │  (30s)         │      │ Loop         │                    │
│  └────────────────┘      └──────────────┘                    │
│                                │                              │
│                                ├──> 1. GetAllPending()        │
│                                ├──> 2. DetectState()          │
│                                ├──> 3. SendMessage()          │
│                                ├──> 4. MarkDelivered()        │
│                                └──> 5. SendAck()              │
│                                                               │
│  ┌────────────────┐      ┌──────────────┐                    │
│  │  Signal        │──────>│ Graceful     │                    │
│  │  Handler       │      │ Shutdown     │                    │
│  └────────────────┘      └──────────────┘                    │
│                                                               │
└──────────────────────────────────────────────────────────────┘
         │                           │
         ▼                           ▼
┌──────────────────┐        ┌──────────────────┐
│  Message Queue   │        │  Session State   │
│  (SQLite + WAL)  │        │  (Manifest)      │
│                  │        │                  │
│  - pending       │        │  - READY         │
│  - delivered     │        │  - THINKING          │
│  - failed        │        │  - COMPACTING    │
│  - attempt_count │        │  - OFFLINE       │
└──────────────────┘        └──────────────────┘
```

### State Machine

```
Message Lifecycle:
  PENDING ──> (session READY) ──> DELIVERED ──> ACK_RECEIVED
     │               │
     │               └──> (send failed) ──> retry ──> PENDING
     │
     └──> (max retries) ──> PERMANENTLY_FAILED ──> DLQ

Session States:
  READY       - Can receive messages (deliver immediately)
  THINKING        - Processing message (defer delivery)
  COMPACTING  - Compacting context (defer delivery)
  OFFLINE     - Session not running (increment retry count)
```

## Message Queue Specification

### Queue Entry Schema

```go
type QueueEntry struct {
    MessageID     string     // UUID v4
    From          string     // Sender session name
    To            string     // Recipient session name
    Message       string     // Message content
    Priority      int        // 0=low, 1=normal (default), 2=high
    Status        string     // "pending", "delivered", "failed"
    AttemptCount  int        // Delivery attempts (0-3)
    CreatedAt     time.Time  // Enqueue timestamp
    DeliveredAt   *time.Time // Delivery timestamp (nil if pending)
    AckRequired   bool       // Require acknowledgment
    AckReceived   bool       // Acknowledgment received
    AckTimeout    *time.Time // Ack timeout deadline (nil if not sent)
}
```

### SQLite Database

**Location**: `~/.agm/queue.db`

**Mode**: WAL (Write-Ahead Logging) for concurrent read/write

**Schema**:
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

## Delivery Algorithm

### Polling Cycle (Every 30 Seconds)

```go
func deliverPending() error {
    // 1. Get all pending messages from queue
    entries := queue.GetAllPending()

    for _, entry := range entries {
        // 2. Resolve recipient session and manifest
        manifest, manifestPath := session.ResolveIdentifier(entry.To)

        // 3. Detect current session state
        currentState := session.DetectState(manifest.Tmux.SessionName)

        // 4. State-aware delivery logic
        switch currentState {
        case StateReady:
            // Deliver message via tmux
            tmux.SendMultiLinePromptSafe(manifest.Tmux.SessionName, entry.Message)

            // Update session state to THINKING
            session.UpdateSessionState(manifestPath, StateBusy, "daemon")

            // Mark as delivered in queue
            queue.MarkDelivered(entry.MessageID)

            // Send acknowledgment (if AckManager configured)
            ackManager.SendAck(entry.MessageID)

        case StateBusy, StateCompacting:
            // Defer delivery (leave in queue)
            continue

        case StateOffline:
            // Increment retry count
            retryLater(entry, "session offline")
        }
    }
}
```

### Retry Logic

```go
func retryLater(entry QueueEntry, reason error) error {
    newAttemptCount := entry.AttemptCount + 1

    if newAttemptCount >= MaxRetries {
        // Permanently failed (3 attempts exceeded)
        queue.MarkPermanentlyFailed(entry.MessageID)
        return fmt.Errorf("max retries exceeded")
    }

    // Increment attempt count
    queue.IncrementAttempt(entry.MessageID)

    // Calculate exponential backoff (for logging)
    backoff := InitialBackoff * 2^(newAttemptCount - 1)
    // 1st retry: 5s, 2nd retry: 10s, 3rd retry: 20s

    // Message stays in queue, retried on next poll cycle
    return nil
}
```

## Message Acknowledgment

### Acknowledgment Protocol

1. **Delivery**: Daemon sends message to session, records delivery timestamp
2. **Set Timeout**: 60-second acknowledgment timeout starts
3. **Wait for Ack**: AckManager blocks on channel waiting for acknowledgment
4. **Timeout Check**: On next poll cycle, daemon checks for timed-out acknowledgments
5. **Requeue or Complete**: Timed-out messages requeued, acknowledged messages removed from queue

### AckManager

```go
type AckManager struct {
    queue       *MessageQueue
    pendingAcks map[string]chan error  // messageID -> ack channel
    mu          sync.RWMutex
}

// WaitForAck blocks until acknowledgment received or timeout
func (am *AckManager) WaitForAck(messageID string, timeout time.Duration) error {
    ackChan := make(chan error, 1)
    am.pendingAcks[messageID] = ackChan

    select {
    case err := <-ackChan:
        return err  // Acknowledged
    case <-time.After(timeout):
        return fmt.Errorf("ack timeout")
    }
}

// SendAck signals acknowledgment for a message
func (am *AckManager) SendAck(messageID string) error {
    if ackChan, ok := am.pendingAcks[messageID]; ok {
        ackChan <- nil  // Success
        queue.MarkAcknowledged(messageID)
        delete(am.pendingAcks, messageID)
    }
}
```

## CLI Commands

### agm daemon start

**Purpose**: Start daemon in background

**Behavior**:
- Checks if daemon already running (via PID file)
- Creates PID file at `~/.agm/daemon.pid`
- Logs to `~/.agm/logs/daemon/daemon.log`
- Runs in background (detached process)

**Example**:
```bash
$ agm daemon start
AGM Daemon starting...
  Base dir: ~/.agm
  Log dir: ~/.agm/logs/daemon
  PID file: ~/.agm/daemon.pid
Daemon started (PID 12345)
```

### agm daemon stop

**Purpose**: Stop running daemon

**Behavior**:
- Reads PID from `~/.agm/daemon.pid`
- Sends SIGTERM to daemon process
- Waits for graceful shutdown (max 10s)
- Removes PID file

**Example**:
```bash
$ agm daemon stop
Stopping daemon (PID 12345)...
Daemon stopped
```

### agm daemon status

**Purpose**: Show daemon and queue status

**Output**:
```bash
$ agm daemon status

AGM Daemon Status
═══════════════════════════════════════════════════════════════

Session Status:
┌────────────────────┬───────────┬──────────┬────────┬──────────┐
│ Session            │ State     │ Queued   │ Failed │ Updated  │
├────────────────────┼───────────┼──────────┼────────┼──────────┤
│ work-session-1     │ READY     │ 0        │ 0      │ 2m ago   │
│ research-session-2 │ THINKING      │ 3        │ 0      │ 30s ago  │
│ archived-session-3 │ OFFLINE   │ 0        │ 2      │ 5d ago   │
└────────────────────┴───────────┴──────────┴────────┴──────────┘

Queue Summary:
  Pending: 3 messages
  Failed:  2 messages
  Total:   5 messages

Daemon: Running (PID 12345)
Poll Interval: 30s
Last Poll: 5s ago
Next Poll: in 25s
```

### agm daemon restart

**Purpose**: Stop then start daemon

**Behavior**: Equivalent to `agm daemon stop && agm daemon start`

## Configuration

### Constants

```go
const (
    PollInterval    = 30 * time.Second  // How often to poll queue
    MaxRetries      = 3                  // Max delivery attempts
    InitialBackoff  = 5 * time.Second   // Starting backoff delay
    AckTimeout      = 60 * time.Second  // Acknowledgment deadline
)
```

### File Locations

- **PID File**: `~/.agm/daemon.pid`
- **Log Directory**: `~/.agm/logs/daemon/`
- **Log File**: `~/.agm/logs/daemon/daemon.log`
- **Queue Database**: `~/.agm/queue.db`

### Daemon Config Struct

```go
type Config struct {
    BaseDir    string                  // ~/.agm
    LogDir     string                  // ~/.agm/logs/daemon
    PIDFile    string                  // ~/.agm/daemon.pid
    Queue      *messages.MessageQueue  // Queue instance
    AckManager *messages.AckManager    // Ack manager instance
    Logger     *log.Logger             // Logger instance
}
```

## Performance Requirements

### Latency Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Queue poll | <100ms | SQLite query with index |
| State detection | <50ms | Manifest read + tmux check |
| Message delivery | <200ms | tmux send-keys command |
| Full delivery cycle | <500ms | End-to-end for single message |

### Throughput

- **Max messages/poll**: 100 messages
- **Max sessions**: 50 concurrent sessions
- **Queue capacity**: 10,000+ messages (SQLite scales well)

### Resource Usage

- **Memory**: ~10MB baseline + ~1KB per queued message
- **CPU**: Minimal (mostly I/O wait on SQLite and tmux)
- **Disk**: Queue database grows ~1KB per message

## Error Handling

### Error Categories

1. **Initialization Errors** (Fatal)
   - PID file already exists (daemon running)
   - Cannot create log directory
   - Cannot open message queue database
   - Action: Exit with error message

2. **Queue Errors** (Non-Fatal)
   - GetAllPending() fails (database locked)
   - MarkDelivered() fails
   - Action: Log error, skip this poll cycle, retry next cycle

3. **Session Errors** (Non-Fatal)
   - Session manifest not found
   - Session state detection fails
   - Action: Increment retry count, continue with next message

4. **Delivery Errors** (Retriable)
   - tmux SendMessage() fails
   - State transition update fails
   - Action: Leave message in queue, retry on next poll

### Logging

**Format**: `[daemon] <timestamp> <message>`

**Levels**:
- **Info**: Daemon start/stop, delivery success, state transitions
- **Warning**: Retry increments, acknowledgment timeouts
- **Error**: Queue failures, delivery failures, unexpected errors

**Example Log**:
```
[daemon] 2026-02-19 14:30:00 Daemon starting...
[daemon] 2026-02-19 14:30:00 Poll interval: 30s
[daemon] 2026-02-19 14:30:30 Processing 3 queued message(s)...
[daemon] 2026-02-19 14:30:30 Session work-session-1 is in state: READY (message msg-abc)
[daemon] 2026-02-19 14:30:30 Delivered message msg-abc to session work-session-1
[daemon] 2026-02-19 14:30:30 Sent acknowledgment for message msg-abc
[daemon] 2026-02-19 14:30:30 Delivery summary: 1 delivered, 0 failed
```

## Shutdown Behavior

### Graceful Shutdown

1. **Signal Received**: SIGTERM or SIGINT caught
2. **Stop Polling**: Cancel context, stop ticker
3. **Wait for Current Cycle**: Let in-flight deliveries complete
4. **Close Resources**: Close queue database connection
5. **Remove PID File**: Delete `~/.agm/daemon.pid`
6. **Exit**: Return 0

**Timeout**: 10 seconds max for graceful shutdown, then force exit

## Security & Privacy

### Security Principles

1. **Local Only**: Daemon runs locally, no network exposure
2. **Read-Write Queue**: Queue requires file permissions (0600)
3. **No Credentials**: No API keys or secrets in queue or logs
4. **Session Isolation**: Only delivers to authorized sessions

### Privacy Guarantees

- **Message Content**: Encrypted at rest if ~/.agm/ is on encrypted filesystem
- **Logs**: Truncate messages to 60 chars in logs (prevent sensitive data exposure)
- **State Tracking**: Only session state (READY/THINKING/etc), not conversation content

## Dependencies

### External Dependencies

1. **Tmux**: Required for message delivery
   - Commands: `send-keys`, `display-message`
   - Version: 2.6+

2. **SQLite**: Embedded database for queue
   - Library: `modernc.org/sqlite` (CGo-free)
   - Mode: WAL (Write-Ahead Logging)

### Internal Dependencies

1. **internal/daemon**: Daemon orchestration logic
2. **internal/messages**: Queue and acknowledgment management
3. **internal/session**: Session state detection and manifest handling
4. **internal/tmux**: Tmux command utilities

## Testing Requirements

### Unit Tests

- Daemon lifecycle (Start, Stop, PID file management)
- Delivery loop logic (state-aware routing)
- Retry logic (attempt count, max retries, exponential backoff)
- Acknowledgment protocol (timeout, requeue)

### Integration Tests

- End-to-end message delivery to real tmux session
- State transitions (READY → THINKING → READY)
- Queue persistence across daemon restarts
- Concurrent message delivery to multiple sessions

### Performance Tests

- Queue poll performance (100 messages)
- Delivery latency (message enqueue to delivery)
- Resource usage (memory, CPU) under load

## Deployment

### Build

```bash
cd ./agm
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
# Start daemon
agm daemon start

# Check status
agm daemon status

# View logs
tail -f ~/.agm/logs/daemon/daemon.log

# Stop daemon
agm daemon stop
```

## Related Documentation

### Architecture Decision Records

- **[ADR-006: Message Queue Architecture](../../docs/adr/ADR-006-message-queue-architecture.md)**: Queue design and implementation
- **[ADR-007: Hook-Based State Detection](../../docs/adr/ADR-007-hook-based-state-detection.md)**: Session state detection strategy
- **[ADR-008: Status Aggregation Pattern](../../docs/adr/ADR-008-status-aggregation.md)**: Status query and aggregation design

### Implementation

- **[internal/daemon/daemon.go](../../internal/daemon/daemon.go)**: Core daemon implementation
- **[internal/messages/queue.go](../../internal/messages/queue.go)**: Message queue implementation
- **[internal/messages/ack.go](../../internal/messages/ack.go)**: Acknowledgment protocol

### Tests

- **[internal/daemon/daemon_test.go](../../internal/daemon/daemon_test.go)**: Unit tests
- **[test/integration/cross_session_test.go](../../test/integration/cross_session_test.go)**: Integration tests

---

**Version**: 1.0
**Last Updated**: 2026-02-19
**Implementation**: Phase 2, Tasks 2.1, 2.3
**Beads**: oss-clnn (Delivery Daemon), oss-ylk4 (Acknowledgment)
