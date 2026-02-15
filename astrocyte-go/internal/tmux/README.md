# Tmux Client Package

This package provides a Go client for interacting with tmux sessions for the Astrocyte daemon.

## Overview

The tmux package provides:
- **Client**: Wrapper for tmux command execution with AGM socket support
- **PaneInfo**: Captured state of a tmux pane with stuck detection logic
- **Pattern Detection**: Identifies stuck sessions using regex patterns

## Components

### Client (`client.go`)

The `Client` struct wraps tmux command execution and handles socket detection.

**Features:**
- Multi-socket support (AGM socket + system default)
- Automatic socket detection for sessions
- Session listing, pane capture, cursor tracking, key sending

**Usage:**

```go
import "github.com/vbonnet/ai-tools/astrocyte/internal/tmux"

// Create client
client := tmux.NewClient()

// List sessions
sessions, err := client.ListSessions()

// Capture pane content
content, err := client.GetPaneContent("session-name")

// Get cursor position
x, y, err := client.GetCursorPosition("session-name")

// Send keys (recovery)
err = client.SendKeys("session-name", "Escape")
err = client.SendKeys("session-name", "C-c")

// Check session exists
if client.HasSession("session-name") {
    // Session exists
}
```

### PaneInfo (`pane.go`)

The `PaneInfo` struct represents a captured tmux pane state with detection methods.

**Features:**
- Last command extraction from pane content
- Permission prompt detection
- Stuck indicator detection (mustering, waiting, completion, etc.)
- Simple stuck session detection logic

**Usage:**

```go
// Capture pane info
pane, err := tmux.CapturePaneInfo(client, "session-name")

// Check if stuck
if pane.IsStuck() {
    reason := pane.GetStuckReason()
    fmt.Printf("Session stuck: %s\n", reason)
}

// Get detailed indicators
indicators := pane.DetectStuckIndicators()
if indicators["zero_token_waiting"] {
    // Session has spinner but no activity
}

// Extract last command
cmd := pane.ExtractLastCommand()

// Detect permission prompts
if pane.DetectPermissionPrompt() {
    // User permission needed
}
```

## Stuck Detection Patterns

The package detects several stuck session indicators:

### Mustering Patterns
Session stuck during initialization:
- `✻ Mustering...`
- `✶ Evaporating...`
- `✢ Mustering...`

### Waiting Patterns
Session stuck with spinner (zero token activity):
- `✶ Thinking...`
- `✢ Processing...`
- `✻ Working...`
- Generic pattern: `[✶✢✻·] verb...`

### Permission Prompts
Session waiting for user permission:
- `Allow ... to ...?`
- `Permission to ...?`
- `(y/n)` or `[y/n]`

### Completion Patterns
Session finished work (NOT stuck):
- `✅` or `✓`
- `Task completed/finished/done`
- `Ready to proceed`
- `What would you like`

### Idle Prompt
Session ready for input (NOT stuck):
- `❯` character at end of output

## Detection Logic

A session is considered stuck if:
1. Has mustering/waiting pattern AND
2. No idle prompt visible AND
3. No completion language present

The `zero_token_waiting` indicator is the most common stuck state, indicating the Claude API is stuck in thinking mode without producing tokens.

## Integration Tests

The package includes comprehensive integration tests that require tmux:

```bash
# Run all tests
go test -v ./internal/tmux/...

# Run only unit tests (no tmux required)
go test -v -short ./internal/tmux/...

# Run integration tests
go test -v -run Integration ./internal/tmux/...
```

Integration tests will be skipped if tmux is not available.

## Performance

The package is optimized for monitoring:
- Pre-compiled regex patterns
- Efficient scrollback capture (500 lines max)
- Minimal tmux command execution

**Benchmarks:**
- `ListSessions`: ~5-10ms per call
- `GetPaneContent`: ~10-20ms per session
- `DetectStuckIndicators`: ~0.1-0.5ms per pane

## Socket Detection

The client supports multiple tmux sockets:

1. **AGM Socket** (`/tmp/agm.sock`): Checked first (priority)
2. **System Default** (`/tmp/tmux-{uid}/default`): Fallback
3. **Empty Path**: Lets tmux use its default socket

Sessions are automatically located across all available sockets.

## Error Handling

All methods return errors for:
- Session not found on any socket
- tmux command execution failure
- Invalid output parsing

Example:
```go
content, err := client.GetPaneContent("session-name")
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Session doesn't exist
    } else {
        // Other error
    }
}
```

## Testing Strategy

1. **Unit Tests**: Pattern matching, content parsing (no tmux)
2. **Integration Tests**: Full client workflow (requires tmux)
3. **Simulation Tests**: Mock scenarios with realistic content
4. **Benchmarks**: Performance testing

Test coverage target: 90%+

## Future Enhancements

- [ ] Support for multiple panes per session
- [ ] Window-level monitoring
- [ ] Session history persistence
- [ ] Custom pattern configuration
- [ ] Remote tmux support (SSH)
