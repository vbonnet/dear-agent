# TUI Event Bus Client

This package provides a WebSocket client for subscribing to AGM event bus events in TUI applications.

## Features

- WebSocket-based real-time event streaming
- Auto-reconnect with exponential backoff
- HTTP polling fallback when WebSocket is unavailable
- Bubble Tea integration for TUI applications
- Thread-safe concurrent access
- Event validation
- Graceful shutdown

## Quick Start

### Basic Usage

```go
import (
    "github.com/vbonnet/ai-tools/agm/internal/tui"
    "github.com/vbonnet/ai-tools/agm/internal/eventbus"
)

// Create client
client := tui.NewEventBusClient("ws://localhost:8080/ws")

// Connect to server
if err := client.Connect("ws://localhost:8080/ws"); err != nil {
    log.Fatal(err)
}
defer client.Close()

// Subscribe to events for a specific session
if err := client.Subscribe("my-session-id"); err != nil {
    log.Fatal(err)
}

// Subscribe to all sessions
if err := client.Subscribe("*"); err != nil {
    log.Fatal(err)
}

// Listen for events
for event := range client.Listen() {
    log.Printf("Received event: %s for session %s", event.Type, event.SessionID)

    // Parse payload based on event type
    switch event.Type {
    case eventbus.EventSessionEscalated:
        var payload eventbus.SessionEscalatedPayload
        if err := event.ParsePayload(&payload); err != nil {
            log.Printf("Failed to parse payload: %v", err)
            continue
        }
        log.Printf("Escalation: %s - %s", payload.EscalationType, payload.Description)

    case eventbus.EventSessionStuck:
        var payload eventbus.SessionStuckPayload
        if err := event.ParsePayload(&payload); err != nil {
            log.Printf("Failed to parse payload: %v", err)
            continue
        }
        log.Printf("Session stuck: %s", payload.Reason)
    }
}
```

### Bubble Tea Integration

```go
import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/vbonnet/ai-tools/agm/internal/tui"
    "github.com/vbonnet/ai-tools/agm/internal/eventbus"
)

type Model struct {
    eventClient   *tui.EventBusClient
    lastEvent     *eventbus.Event
    notifications []string
    connected     bool
}

func (m *Model) Init() tea.Cmd {
    // Connect to event bus
    m.eventClient.Connect("ws://localhost:8080/ws")
    m.eventClient.Subscribe("*")

    // Start listening for events
    return tea.Batch(
        tui.WaitForEventCmd(m.eventClient),
        tui.CheckConnectionCmd(m.eventClient),
    )
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tui.EventReceivedMsg:
        // Handle received event
        m.lastEvent = msg.Event
        m.notifications = append(m.notifications, formatEvent(msg.Event))

        // Continue listening
        return m, tui.WaitForEventCmd(m.eventClient)

    case tui.ConnectionStatusMsg:
        // Handle connection status change
        m.connected = msg.Connected

        // Check again later
        return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
            return tui.CheckConnectionCmd(m.eventClient)()
        })
    }

    return m, nil
}
```

## Event Types

The client supports the following event types:

- `eventbus.EventSessionEscalated` - Session requires human intervention
- `eventbus.EventSessionStuck` - Session appears stuck (no output)
- `eventbus.EventSessionRecovered` - Session recovered from stuck state
- `eventbus.EventSessionStateChange` - Session state transition
- `eventbus.EventSessionCompleted` - Session completed successfully

See `internal/eventbus/schema.go` for complete event type definitions and payloads.

## Configuration

### Environment Variables

- `AGM_EVENTBUS_PORT` - Event bus server port (default: 8080)
- `AGM_EVENTBUS_MAX_CLIENTS` - Maximum concurrent clients (default: 100)

### Client Configuration

The client uses the following default values:

- **Reconnect Delay**: 1s (initial), up to 30s (max)
- **Backoff Multiplier**: 2x
- **HTTP Poll Interval**: 5s (when WebSocket unavailable)
- **Event Channel Buffer**: 256 events

## Auto-Reconnect

The client automatically reconnects when the connection is lost:

1. Initial delay: 1 second
2. Each retry doubles the delay (exponential backoff)
3. Maximum delay: 30 seconds
4. Reconnects indefinitely until `Close()` is called
5. Automatically resubscribes to the same session after reconnect

## HTTP Fallback

If the WebSocket connection fails, the client automatically falls back to HTTP polling:

- Polls every 5 seconds
- Uses `/api/events?session_id={id}&since={timestamp}` endpoint
- Note: The HTTP endpoint must be implemented separately (stub included)

## Thread Safety

All client methods are thread-safe and can be called from multiple goroutines:

- `Connect()` - Connect to server
- `Subscribe()` - Subscribe to session events
- `Listen()` - Get event channel (safe to read from multiple goroutines)
- `IsConnected()` - Check connection status
- `Close()` - Graceful shutdown

## Testing

The package includes comprehensive tests with 73.9% coverage:

```bash
go test ./internal/tui/... -cover -v
```

Test coverage includes:

- WebSocket connection lifecycle
- Event reception and validation
- Auto-reconnect logic
- HTTP fallback
- Concurrent access
- Event channel overflow handling
- Bubble Tea integration

## Example TUI Application

See `example_model.go` for a complete example of a Bubble Tea TUI application that:

- Connects to the event bus
- Displays real-time notifications
- Shows connection status
- Handles different event types with visual indicators
- Supports keyboard shortcuts (c to clear, q to quit)

Run the example:

```go
model := tui.NewExampleModel("ws://localhost:8080/ws")
p := tea.NewProgram(model)
if err := p.Start(); err != nil {
    log.Fatal(err)
}
```

## Architecture

### EventBusClient

The main client struct that manages the WebSocket connection:

```go
type EventBusClient struct {
    url           string                    // WebSocket URL
    sessionID     string                    // Subscribed session ID
    conn          *websocket.Conn           // WebSocket connection
    events        chan *eventbus.Event      // Event channel
    done          chan struct{}             // Shutdown signal
    reconnect     bool                      // Enable auto-reconnect
    isConnected   bool                      // Connection status
    httpFallback  bool                      // Using HTTP polling
}
```

### Message Types

Bubble Tea message types for TUI integration:

- `EventReceivedMsg` - Contains received event
- `ConnectionStatusMsg` - Contains connection status and error

### Commands

Bubble Tea commands:

- `WaitForEventCmd(client)` - Waits for next event (blocking)
- `CheckConnectionCmd(client)` - Checks connection status (non-blocking)

## Best Practices

1. **Always defer Close()** to ensure graceful shutdown
2. **Handle all event types** in your switch statement
3. **Validate events** using `event.Validate()` if needed
4. **Parse payloads** with error handling
5. **Use buffered channels** when forwarding events
6. **Check connection status** before sending critical commands
7. **Subscribe early** in your application lifecycle

## Troubleshooting

### Connection Fails Immediately

- Check that the event bus server is running
- Verify the WebSocket URL is correct
- Check firewall/network settings
- Client will automatically fall back to HTTP polling

### Events Not Received

- Verify you've called `Subscribe()` with correct session ID
- Check that the session ID matches events being broadcast
- Ensure you're reading from the `Listen()` channel
- Check event channel buffer isn't full (256 events)

### Reconnect Loops

- Check server availability
- Verify network connectivity
- Look for server-side connection limits
- Check server logs for rejected connections

### High Memory Usage

- Ensure you're reading from the event channel
- Don't accumulate too many events in memory
- Use a bounded notification list (see example)
- Consider implementing event archiving

## Future Improvements

Potential enhancements (not currently implemented):

- [ ] HTTP endpoint implementation for polling fallback
- [ ] Configurable reconnect parameters
- [ ] Event filtering on client side
- [ ] Event replay from timestamp
- [ ] Metrics and monitoring hooks
- [ ] Rate limiting for event processing
- [ ] Event batching for efficiency
