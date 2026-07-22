# internal/send

Multi-recipient message delivery system for AGM sessions.

## Overview

This package provides the core functionality for sending messages to one or more AGM sessions with sequential delivery and per-recipient error handling.

## Components

### Core Files

- **`multi_recipient.go`**
  - Recipient parsing and resolution
  - Glob pattern matching
  - Session validation and deduplication

- **`delivery.go`**
  - Sequential message delivery under the shared tmux mutation boundary
  - Caller-context propagation and cancellation
  - Per-recipient error isolation

- **`result_collector.go`**
  - Delivery result aggregation
  - Color-coded reporting
  - Success/failure tracking

### Test Files

- **`multi_recipient_test.go`**
- **`delivery_test.go`**
- **`result_collector_test.go`**

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

// Execute sequential delivery with timeout. The CLI delivery function routes
// each job through shared ops atomic readiness and exact-pane input.
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()
results := send.SequentialDeliver(ctx, jobs, deliveryFunc)

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

### Sequential Delivery

- One recipient is processed at a time because tmux mutation is serialized
- Each registered CLI harness uses shared ops atomic readiness and delivery
- The exact verified pane remains pinned for each input operation
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
type DeliveryFunc func(ctx context.Context, job *DeliveryJob) error
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
go test ./agm/internal/send -v
```

Run with coverage:
```bash
go test ./agm/internal/send -cover
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

## Delivery Policy

- Recipients are processed in resolved order.
- Each delivery finishes its readiness check and tmux mutation before the next begins.
- Cancellation stops undispatched jobs and records a result for each recipient.
- A recipient failure is reported without preventing later recipients from being attempted.

## See Also

- `cmd/agm/send_msg.go` - Command integration
- `internal/dolt/` - Session storage adapter
- `internal/messages/` - Message logging

## License

Part of the AGM (AI-Generated Multisession) project.
