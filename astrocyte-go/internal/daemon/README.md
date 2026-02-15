# Daemon Package

This package provides stuck session detection logic for the Astrocyte daemon.

## Overview

The daemon package builds on top of the tmux client to provide:
- **Session History Tracking**: Cursor position tracking over time
- **Stuck Session Detection**: Multi-indicator analysis for hang detection
- **Configurable Thresholds**: Timeout customization per detection type

## Components

### SessionHistory (`detector.go`)

Tracks cursor positions over time to detect frozen sessions.

**Features:**
- Rolling window of cursor snapshots
- Configurable history size
- Freeze detection based on position stability

**Usage:**

```go
import "github.com/vbonnet/ai-tools/astrocyte/internal/daemon"

// Create history tracker
history := daemon.NewSessionHistory(10) // Keep 10 snapshots

// Add cursor snapshots over time
history.AddSnapshot(10, 20, time.Now())
time.Sleep(1 * time.Minute)
history.AddSnapshot(10, 20, time.Now()) // Same position

// Check if frozen
if history.IsCursorFrozen(5 * time.Minute) {
    // Cursor hasn't moved in 5 minutes
}
```

### StuckSessionDetector (`detector.go`)

Comprehensive stuck session detector combining multiple indicators.

**Features:**
- Multi-session tracking
- Configurable timeout thresholds
- Integration with tmux.PaneInfo
- Detailed stuck session information

**Usage:**

```go
// Create detector with default thresholds
detector := daemon.NewStuckSessionDetector()

// Customize thresholds
detector.MusteringTimeout = 20         // 20 minutes
detector.ZeroTokenWaitingTimeout = 15  // 15 minutes
detector.CursorFrozenTimeout = 30      // 30 minutes
detector.PermissionPromptDuration = 10 // 10 minutes

// Track session cursor position
detector.TrackSession("session-name", cursorX, cursorY)

// Detect if stuck
pane := &tmux.PaneInfo{
    SessionName: "session-name",
    Content:     "✶ Thinking...",
    CursorX:     10,
    CursorY:     20,
    CapturedAt:  time.Now(),
}

stuck, reason := detector.IsSessionStuck(pane)
if stuck {
    fmt.Printf("Session stuck: %s\n", reason)
}

// Get comprehensive stuck info
info := detector.DetectStuckSession(pane)
if info != nil {
    fmt.Println(info.String())
    // Access details
    fmt.Printf("Reason: %s\n", info.Reason)
    fmt.Printf("Last command: %s\n", info.LastCommand)
    fmt.Printf("Indicators: %+v\n", info.Indicators)
}
```

## Detection Algorithm

The detector combines multiple indicators to determine if a session is stuck:

### 1. Mustering Timeout
**Pattern**: `✻ Mustering...` without idle prompt
**Threshold**: 20 minutes (default)
**Reason**: `stuck_mustering`

Session is stuck during initialization phase.

### 2. Zero Token Waiting
**Pattern**: Waiting spinner without idle prompt
**Threshold**: 15 minutes (default)
**Reason**: `stuck_zero_token_waiting`

Most common stuck state - Claude API stuck in thinking mode with no token production.

### 3. Permission Prompt
**Pattern**: Permission prompt patterns (`y/n`, `Allow...?`)
**Threshold**: 10 minutes (default)
**Reason**: `stuck_permission_prompt`

Session waiting for user to respond to permission request.

### 4. Cursor Frozen
**Pattern**: Cursor unmoved for duration
**Threshold**: 30 minutes (default)
**Reason**: `cursor_frozen`

UI completely unresponsive (very conservative threshold).

**False Positive Prevention:**
- Ignores frozen cursor if completion language present
- Ignores frozen cursor if idle prompt visible

### 5. General Waiting
**Pattern**: Waiting indicator without completion/idle
**Threshold**: N/A (detected immediately)
**Reason**: `stuck_waiting`

Generic stuck state fallback.

## Stuck Reasons

| Reason | Description | Typical Cause |
|--------|-------------|---------------|
| `stuck_mustering` | Stuck during session initialization | API timeout, network issue |
| `stuck_zero_token_waiting` | Stuck in thinking with no output | API stall, 0-token bug |
| `stuck_permission_prompt` | Waiting for user permission | User didn't respond |
| `cursor_frozen` | UI completely unresponsive | Complete freeze, deadlock |
| `stuck_waiting` | Generic waiting state | Various causes |

## SessionStuckInfo

Comprehensive stuck session information:

```go
type SessionStuckInfo struct {
    SessionName string              // Session identifier
    Reason      string              // Stuck reason code
    Indicators  map[string]bool     // All detected indicators
    LastCommand string              // Last executed command
    CursorX     int                 // Cursor X position
    CursorY     int                 // Cursor Y position
    DetectedAt  time.Time          // When stuck state was detected
}
```

## Integration with Enforcement

The detector integrates with the enforcement package for violation detection:

```go
import (
    "github.com/vbonnet/ai-tools/astrocyte/internal/daemon"
    "github.com/vbonnet/ai-tools/astrocyte/pkg/enforcement"
)

// Load enforcement patterns
patterns, _ := enforcement.LoadPatterns("patterns.yaml")
enforcementDetector, _ := enforcement.NewDetector(patterns)

// Create stuck detector
stuckDetector := daemon.NewStuckSessionDetector()

// Check for both stuck state AND violations
pane := capturePane(session)

// Stuck detection
stuckInfo := stuckDetector.DetectStuckSession(pane)

// Violation detection
violation, _ := enforcementDetector.Detect(pane.Content)

if stuckInfo != nil {
    // Session is stuck
    handleStuck(stuckInfo)
}

if violation != nil {
    // Violation detected
    handleViolation(violation)
}
```

## Testing

The package includes comprehensive tests:

### Unit Tests
Test individual components without external dependencies:
```bash
go test -v ./internal/daemon/
```

### Integration Tests
Test full workflows with simulated sessions:
```bash
go test -v -run Integration ./internal/daemon/
```

### Benchmarks
Performance testing:
```bash
go test -bench=. ./internal/daemon/
```

**Coverage Target**: 90%+

## Configuration Best Practices

### Conservative Defaults
Default timeouts are intentionally conservative to minimize false positives:

```go
MusteringTimeout:         20 minutes  // Only genuine hangs
ZeroTokenWaitingTimeout:  15 minutes  // API truly stuck
CursorFrozenTimeout:      30 minutes  // Very conservative
PermissionPromptDuration: 10 minutes  // User might be AFK
```

### Session-Specific Tuning
Different session types may need different thresholds:

```go
// Orchestrator sessions (monitor multiple other sessions)
orchestratorDetector := daemon.NewStuckSessionDetector()
orchestratorDetector.CursorFrozenTimeout = 60 // 1 hour - very long idle OK

// Single task sessions
taskDetector := daemon.NewStuckSessionDetector()
taskDetector.CursorFrozenTimeout = 15 // 15 minutes - expect activity

// Interactive sessions
interactiveDetector := daemon.NewStuckSessionDetector()
interactiveDetector.CursorFrozenTimeout = 10 // 10 minutes - user thinking time
```

### False Positive Prevention
The detector actively prevents false positives:

1. **Completion Check**: Never mark as stuck if completion language present
2. **Idle Prompt Check**: Never mark as stuck if `❯` visible
3. **Cursor Frozen**: Only triggers if no other positive indicators
4. **Conservative Timeouts**: Better to miss a hang than interrupt work

## Example: Full Monitoring Loop

```go
func monitorSessions(client *tmux.Client, detector *daemon.StuckSessionDetector) {
    ticker := time.NewTicker(60 * time.Second) // Check every minute
    defer ticker.Stop()

    for {
        <-ticker.C

        // Get all sessions
        sessions, err := client.ListSessions()
        if err != nil {
            log.Printf("Error listing sessions: %v", err)
            continue
        }

        // Check each session
        for _, sessionName := range sessions {
            // Capture pane state
            pane, err := tmux.CapturePaneInfo(client, sessionName)
            if err != nil {
                log.Printf("Error capturing %s: %v", sessionName, err)
                continue
            }

            // Track cursor movement
            detector.TrackSession(sessionName, pane.CursorX, pane.CursorY)

            // Detect stuck state
            info := detector.DetectStuckSession(pane)
            if info != nil {
                log.Printf("STUCK: %s", info.String())
                // Trigger recovery...
            }
        }
    }
}
```

## Future Enhancements

- [ ] Adaptive thresholds based on session history
- [ ] Session-type detection (orchestrator vs task vs interactive)
- [ ] Per-session threshold overrides
- [ ] Incident history and pattern analysis
- [ ] Machine learning-based stuck prediction
- [ ] Integration with remote monitoring/alerts
