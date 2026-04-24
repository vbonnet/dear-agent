# OpenCode SSE Adapter

This package implements Task 1.1 of the Multi-Agent Integration Specification: SSE Client Package for OpenCode SSE Adapter.

## Files Implemented

### Core Implementation

- **sse_adapter.go**: Main SSE adapter with connection management, auto-reconnect, and health monitoring
- **sse_adapter_test.go**: Comprehensive test suite with >80% coverage
- **event_parser.go**: OpenCode event parser mapping SSE events to AGM states
- **event_parser_test.go**: Event parser test suite
- **publisher.go**: EventBus publisher with backpressure handling and circuit breaker
- **publisher_test.go**: Publisher test suite
- **lifecycle.go**: Session lifecycle manager (Adapter interface implementation)
- **lifecycle_test.go**: Lifecycle manager test suite

### Documentation

- **doc.go**: Package documentation
- **ARCHITECTURE.md**: Component architecture guide
- **README.md**: This file

## Features

### 1. HTTP Client with Proper Timeouts

- Connection timeout: 10s
- TLS handshake timeout: 10s
- Response header timeout: 10s
- Keep-alive: 30s
- Idle connection timeout: 90s

### 2. SSE Connection Management

- Connects to `GET /event` endpoint
- Validates `Content-Type: text/event-stream`
- Parses SSE `data:` lines using `bufio.Scanner`
- Handles comment lines (`:`) as heartbeats
- Processes event lines (`event:`)

### 3. Auto-Reconnect with Exponential Backoff

- Initial delay: 1s (configurable)
- Maximum delay: 30s (configurable)
- Backoff multiplier: 2x (configurable)
- Backoff sequence: 1s → 2s → 4s → 8s → 16s → 30s → 30s...
- Unlimited retries by default
- Optional circuit breaker with configurable max retries

### 4. Health Check

- Heartbeat-based health monitoring (separate from event timestamps)
- Prevents false positives for idle sessions
- Reports connection status, last event, and last heartbeat
- Includes metadata (server URL, session ID)

### 5. Graceful Shutdown

- Context cancellation support
- Proper cleanup of HTTP connections
- WaitGroup for goroutine synchronization
- Timeout handling on shutdown

### 6. Thread-Safe Connection Status

- Atomic operations for connection state
- Atomic values for timestamps
- Mutex protection for critical sections
- No race conditions

## API

### Types

```go
type SSEAdapter struct { /* ... */ }
type Config struct {
    ServerURL string
    SessionID string
    Reconnect ReconnectConfig
}
type ReconnectConfig struct {
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   int
    MaxRetries   int
}
type HealthStatus struct {
    Connected     bool
    Error         error
    LastEvent     time.Time
    LastHeartbeat time.Time
    Metadata      map[string]interface{}
}
```

### Interface (Adapter Pattern)

```go
func NewSSEAdapter(parser *EventParser, publisher *Publisher, config Config) *SSEAdapter
func (a *SSEAdapter) Start(ctx context.Context) error
func (a *SSEAdapter) Stop(ctx context.Context) error
func (a *SSEAdapter) Health() HealthStatus
func (a *SSEAdapter) Name() string
```

### EventBusPublisher Interface

```go
type EventBusPublisher interface {
    Broadcast(event *eventbus.Event)
}
```

## Tests

### Test Coverage

The test suite includes:

1. **Connection Tests**
   - TestSSEAdapter_Connect: Successful connection
   - TestSSEAdapter_ConnectionFailure: Handling 503 errors
   - TestSSEAdapter_InvalidContentType: Rejecting wrong content types

2. **Reconnect Tests**
   - TestSSEAdapter_AutoReconnect: Exponential backoff logic
   - TestSSEAdapter_CircuitBreaker: Max retries enforcement

3. **Shutdown Tests**
   - TestSSEAdapter_GracefulShutdown: Clean connection close
   - TestSSEAdapter_ContextCancellation: Context handling

4. **Health Tests**
   - TestSSEAdapter_HeartbeatTracking: Heartbeat vs event timestamps
   - TestSSEAdapter_HealthMetadata: Metadata reporting

5. **Lifecycle Tests**
   - TestSSEAdapter_MultipleStartStop: Multiple start/stop cycles
   - TestSSEAdapter_Name: Adapter naming

6. **Benchmark Tests**
   - BenchmarkSSEAdapter_EventProcessing: Performance testing

### Running Tests

```bash
cd main/agm
go test ./internal/monitor/opencode -v
go test ./internal/monitor/opencode -run TestSSEAdapter_Connect
go test ./internal/monitor/opencode -bench=.
```

## Integration

### With EventBus

The adapter uses dependency injection with separate parser and publisher components:

```go
// Create components
parser := opencode.NewEventParser()
publisher := opencode.NewPublisher(eventBus, "my-session", adapterController)
config := opencode.Config{
    ServerURL: "http://localhost:4096",
    SessionID: "my-session",
    Reconnect: opencode.DefaultReconnectConfig(),
}

// Create SSE adapter
adapter := opencode.NewSSEAdapter(parser, publisher, config)
```

### With Lifecycle Adapter (Recommended)

Use the top-level Adapter for complete lifecycle management:

```go
// Create adapter with all components
adapter, err := opencode.NewAdapter(eventBus, opencode.Config{
    Enabled:        true,
    ServerURL:      "http://localhost:4096",
    SessionID:      "my-session",
    Reconnect:      opencode.DefaultReconnectConfig(),
    FallbackTmux:   true,
    HealthProbeURL: "/health",
    HealthTimeout:  5 * time.Second,
})

// Start with health probe
if err := adapter.Start(ctx); err != nil {
    // Handle startup failure
}
```

## Error Handling

### Typed Errors

- `ErrNotConnected`: Not connected to SSE endpoint
- `ErrInvalidContentType`: Wrong content-type received
- `ErrConnectionFailed`: Connection attempt failed
- `ErrCircuitBreakerOpen`: Circuit breaker activated

### Error Propagation

All errors are wrapped with context using `fmt.Errorf("%w", err)` for proper error chain handling.

## Task 1.4 Completion: Session Lifecycle Manager

### Implementation

The **lifecycle.go** file implements the top-level Adapter that coordinates all components:

#### Key Features

1. **Adapter Interface Implementation**
   - `Start(ctx)`: Health probe + SSE client startup
   - `Stop(ctx)`: Graceful shutdown with timeout
   - `Health()`: Delegates to SSE client health status
   - `Name()`: Returns "opencode-sse"

2. **Component Coordination**
   - Creates and wires SSE client, event parser, and publisher
   - Manages configuration with sensible defaults
   - Validates configuration on creation

3. **Health Probe**
   - Probes `/health` endpoint (configurable)
   - 5-second timeout (configurable)
   - Fails fast if server unreachable
   - Optional fallback to tmux monitoring

4. **Session ID Mapping**
   - `SessionMapper` tracks OpenCode ID ↔ AGM ID
   - Thread-safe with RWMutex
   - Support for Register, Lookup, Remove, Clear operations
   - Concurrent access tested

5. **Graceful Shutdown**
   - Propagates context cancellation to SSE client
   - Waits for goroutines with timeout
   - Cleans up session mappings

#### API

```go
// Top-level adapter
type Adapter struct {
    sseClient *SSEAdapter
    parser    *EventParser
    publisher *Publisher
    config    AdapterConfig
    mapper    *SessionMapper
}

// Configuration
type AdapterConfig struct {
    Enabled         bool
    ServerURL       string
    SessionID       string
    Reconnect       ReconnectConfig
    FallbackTmux    bool
    HealthProbeURL  string
    HealthTimeout   time.Duration
}

// Session mapper
type SessionMapper struct {
    // Thread-safe mapping: opencodeID → agmSessionID
}

// Constructor
func NewAdapter(eventBus EventBusPublisher, config AdapterConfig) (*Adapter, error)

// Adapter interface
func (a *Adapter) Start(ctx context.Context) error
func (a *Adapter) Stop(ctx context.Context) error
func (a *Adapter) Health() HealthStatus
func (a *Adapter) Name() string
```

#### Test Coverage

- ✅ Successful adapter creation with valid configuration
- ✅ Configuration validation (nil eventBus, empty serverURL, empty sessionID)
- ✅ Default configuration values applied
- ✅ Health probe success (200 OK)
- ✅ Health probe failure (500, 404, 503)
- ✅ Health probe timeout
- ✅ Start failure when health probe fails
- ✅ Start with fallback to tmux on health probe failure
- ✅ Graceful shutdown and cleanup
- ✅ Health status delegation to SSE client
- ✅ Session mapper operations (register, lookup, remove, clear)
- ✅ Session mapper concurrent access

### Next Steps

The following components remain for future implementation:

1. **Integration Tests**: E2E tests with real OpenCode server
2. **Daemon Integration**: Wire adapter into daemon startup/shutdown
3. **Metrics Integration**: Replace placeholder incrementMetric() calls

## References

- ARCHITECTURE.md: Detailed component architecture
- MULTI-AGENT-INTEGRATION-SPEC.md: Overall integration specification
- SSE Specification: https://html.spec.whatwg.org/multipage/server-sent-events.html

## Task 1.3 Completion: EventBus Publisher

### Implementation

The **publisher.go** file implements the EventBus Publisher component that publishes parsed OpenCode events to AGM's EventBus.

#### Key Features

1. **Sequence Numbering**
   - Monotonic, thread-safe sequence counter using `atomic.Uint64`
   - Prevents race conditions and event reordering
   - Included in payload as per P0 fixes from SPEC

2. **Metadata Enrichment**
   - Adds `source: "opencode-sse"` to all events
   - Adds `agent: "opencode"` to all events
   - Preserves event-specific metadata from parser

3. **EventBus Integration**
   - Creates `eventbus.Event` with `SessionStateChange` type
   - Uses non-blocking `Broadcast()` method
   - Follows established pattern from Astrocyte watcher

4. **Backpressure Handling**
   - Retry logic with exponential backoff (100ms, 200ms, 400ms)
   - Handles event creation errors (validation, marshaling)
   - Tracks metrics for backpressure delays

5. **Circuit Breaker**
   - Stops adapter after 10 consecutive publish failures
   - Asynchronous adapter stop to avoid deadlocks
   - Resets failure counter on successful publish
   - Tracks circuit breaker trip metrics

6. **Error Handling**
   - Comprehensive logging (INFO, WARN, ERROR, CRITICAL)
   - Metrics tracking (publish failures, backpressure delays, circuit breaker trips)
   - Nil event validation

#### API

```go
// Publisher publishes parsed events to EventBus
type Publisher struct {
    eventBus     EventBusPublisher
    sessionID    string
    sequence     atomic.Uint64
    failureCount atomic.Uint64
    adapter      AdapterController
}

// Interfaces
type EventBusPublisher interface {
    Broadcast(event *eventbus.Event)
}

type AdapterController interface {
    Stop(ctx context.Context) error
}

// Event types
type AGMEvent struct {
    State     string
    Timestamp int64
    Metadata  map[string]interface{}
}

type SessionStateChangeEvent struct {
    SessionID string
    State     string
    Timestamp int64
    Sequence  uint64                 // P0 fix: monotonic sequence
    Source    string                 // "opencode-sse"
    Agent     string                 // "opencode"
    Metadata  map[string]interface{}
}

// Constructor and methods
func NewPublisher(eventBus EventBusPublisher, sessionID string, adapter AdapterController) *Publisher
func (p *Publisher) Publish(event *AGMEvent) error
func (p *Publisher) PublishWithBackpressure(event *AGMEvent) error
func (p *Publisher) GetSequence() uint64
func (p *Publisher) GetFailureCount() uint64
```

#### Event Schema

Published events follow the `eventbus.Event` structure:

```json
{
  "type": "session.state_change",
  "timestamp": "2026-03-06T14:23:00Z",
  "session_id": "my-opencode-session",
  "payload": {
    "session_id": "my-opencode-session",
    "state": "THINKING",
    "timestamp": 1709654321,
    "sequence": 42,
    "source": "opencode-sse",
    "agent": "opencode",
    "metadata": {
      "event_type": "tool.execute.before",
      "tool_name": "Write"
    }
  }
}
```

#### Test Coverage

- ✅ Successful event publishing
- ✅ Sequence number monotonicity (concurrent and sequential)
- ✅ Nil event error handling
- ✅ Backpressure retry logic (with valid events)
- ✅ Circuit breaker activation (10 failures)
- ✅ Circuit breaker reset on success
- ✅ Concurrent publishing (thread-safety)
- ✅ Getter methods (sequence, failure count)

#### Metrics

The publisher tracks these metrics (placeholder implementation):

- `opencode.eventbus.publish_failures`: Total publish failures
- `opencode.eventbus.backpressure_delays`: Number of retry delays
- `opencode.eventbus.circuit_breaker_trips`: Circuit breaker activations

#### Integration

The publisher integrates with the SSE adapter and event parser:

```go
// In SSE adapter event handler
func (a *SSEAdapter) handleEvent(data []byte) {
    // Parse event
    agmEvent, err := a.parser.Parse(data)
    if err != nil {
        log.Printf("Parse error: %v", err)
        return
    }

    // Publish with backpressure handling
    if err := a.publisher.PublishWithBackpressure(agmEvent); err != nil {
        log.Printf("Publish error: %v", err)
        // Error already logged with metrics
    }
}
```

### Backpressure Design Note

The EventBus `Broadcast()` method is non-blocking and drops events when the channel is full (default capacity: 256 events). This design choice prioritizes system stability over guaranteed delivery:

- **Pros**: Never blocks producers, prevents cascade failures
- **Cons**: Events can be dropped under high load

The publisher's backpressure handling primarily protects against:
1. Event creation errors (validation, marshaling)
2. Consecutive failures triggering circuit breaker
3. Provides retry semantics for future blocking implementations

For critical events that cannot be lost, future enhancements could include:
- Dead letter queue for dropped events
- Persistent event log for replay
- Blocking publish option with timeout

## Bead

oss-xvin (Task 1.3)
oss-t5h3 (Task 1.1, 1.2, 1.4)
