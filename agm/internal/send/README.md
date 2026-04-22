# internal/send

Multi-recipient message delivery system for AGM sessions.

## Overview

This package provides the core functionality for sending messages to one or more AGM sessions with parallel execution, comprehensive error handling, and 100% backward compatibility.

## Components

### Core Files

- **`multi_recipient.go`** (~240 lines)
  - Recipient parsing and resolution
  - Glob pattern matching
  - Session validation and deduplication

- **`delivery.go`** (~100 lines)
  - Parallel message delivery with worker pool
  - Concurrency control (max 5 workers)
  - Per-recipient error isolation

- **`result_collector.go`** (~130 lines)
  - Delivery result aggregation
  - Color-coded reporting
  - Success/failure tracking

### Test Files

- **`multi_recipient_test.go`** (~400 lines, 29 tests)
- **`delivery_test.go`** (~350 lines, 10 tests)
- **`result_collector_test.go`** (~400 lines, 15 tests)

**Total: 54 tests, all passing**

## Usage

### From cmd/agm/send_msg.go

```go
// Parse recipients
spec, err := send.ParseRecipients(args, msgTo, msgWorkspace)
if err != nil {
    return err
}

// Resolve to actual sessions
resolver := &doltSessionResolver{adapter: doltAdapter}
resolvedSpec, err := send.ResolveRecipients(spec, resolver)
if err != nil {
    return err
}

// Create delivery jobs
jobs := []*send.DeliveryJob{
    {
        Recipient: "session1",
        Sender: "sender-name",
        MessageID: "msg-001",
        FormattedMessage: "Hello, session!",
        ShouldInterrupt: false,
    },
}

// Execute parallel delivery with timeout
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()
results := send.ParallelDeliver(ctx, jobs, deliveryFunc)

// Generate report
report := send.GenerateReport(results)
report.PrintReport()
```

## Features

### Recipient Parsing

Supports multiple input formats:
- **Single direct**: `session1`
- **Comma-separated**: `session1,session2,session3`
- **Glob patterns**: `*research*`, `test-*`
- **Wildcard**: `*` (all sessions)

### Parallel Delivery

- Worker pool with max 5 concurrent goroutines
- Semaphore-based concurrency control
- Channel-based result collection
- Per-recipient error isolation

### Error Handling

- One failure doesn't block others
- Detailed error reporting per recipient
- Clear success/failure counts
- Color-coded output

### Output Example

```
Sent to 3 recipients (2 succeeded, 1 failed) [1.2s]

Success (2):
  ✓ session1 [ID: msg-123] [0.4s]
  ✓ session2 [ID: msg-124] [0.5s]

Failed (1):
  ✗ session3 [Error: session not found] [0.3s]
```

## Architecture

### Interface Design

```go
// SessionResolver enables dependency injection and testing
type SessionResolver interface {
    ResolveIdentifier(identifier string) (*manifest.Manifest, error)
    ListAllSessions() ([]*manifest.Manifest, error)
}

// DeliveryFunc allows custom delivery implementations
type DeliveryFunc func(job *DeliveryJob) error
```

### Key Types

```go
type RecipientSpec struct {
    Raw        string   // Original input
    Type       string   // "direct", "comma_list", "glob"
    Recipients []string // Resolved session names
}

type DeliveryJob struct {
    Recipient        string
    Sender           string
    MessageID        string
    FormattedMessage string
    ShouldInterrupt  bool
}

type DeliveryResult struct {
    Recipient string
    Success   bool
    Error     error
    Duration  time.Duration
    MessageID string
    Method    string
}

type DeliveryReport struct {
    TotalRecipients int
    SuccessCount    int
    FailureCount    int
    Results         []*DeliveryResult
    TotalDuration   time.Duration
}
```

## Testing

Run tests:
```bash
go test ./internal/send/... -v
```

Run with coverage:
```bash
go test ./internal/send/... -cover
```

## Integration

### With Dolt Adapter

The `dolt.Adapter` is wrapped via `doltSessionResolver` to implement the `SessionResolver` interface:

```go
type doltSessionResolver struct {
    adapter *dolt.Adapter
}

func (r *doltSessionResolver) ResolveIdentifier(id string) (*manifest.Manifest, error) {
    return r.adapter.ResolveIdentifier(id)
}

func (r *doltSessionResolver) ListAllSessions() ([]*manifest.Manifest, error) {
    filter := &dolt.SessionFilter{Lifecycle: ""} // Active only
    return r.adapter.ListSessions(filter)
}
```

## Performance

### Parallel vs. Sequential

- **Sequential**: 3 recipients × 500ms = 1.5s
- **Parallel**: max(500ms) + overhead ≈ 600ms
- **Speedup**: ~2.5x

### Concurrency Control

- Max 5 workers prevents resource exhaustion
- Handles any number of recipients gracefully
- Bounded memory usage

## Future Enhancements

1. **Workspace filtering**: Complete implementation
2. **API-based delivery**: Support for OpenAI/GPT agents
3. **Progress reporting**: Live progress for large lists
4. **Delivery retries**: Automatic retry with backoff

## See Also

- `PHASE2_IMPLEMENTATION.md` - Detailed implementation guide
- `cmd/agm/send_msg.go` - Command integration
- `internal/dolt/` - Session storage adapter
- `internal/messages/` - Message logging

## License

Part of the AGM (AI-Generated Multisession) project.
