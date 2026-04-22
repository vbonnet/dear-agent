# Telemetry Public API - Specification

## Overview

The `pkg/telemetry` package provides a public API for external modules to implement telemetry event listeners. This package re-exports types from `internal/telemetry` to enable integration without violating Go's internal package visibility rules.

## Purpose

**Primary Goal**: Enable external modules (like ai-tools, plugins) to receive telemetry events from Engram core without accessing internal packages.

**Key Capabilities**:
- Implement custom EventListener interface
- Receive filtered telemetry events asynchronously
- Access severity levels and event types
- Integrate with Engram's telemetry system

## Functional Requirements

### FR-1: Public Type Re-exports

The package SHALL re-export essential types from internal/telemetry:

- **FR-1.1**: EventListener interface for implementing listeners
- **FR-1.2**: Event struct for event data
- **FR-1.3**: Level type for severity levels
- **FR-1.4**: Level constants (LevelInfo, LevelWarn, LevelError, LevelCritical)

### FR-2: EventListener Interface

External modules SHALL be able to implement EventListener:

- **FR-2.1**: OnEvent(event *Event) error - Process incoming events
- **FR-2.2**: MinLevel() Level - Specify minimum severity level
- **FR-2.3**: Thread-safe implementation required (called in goroutines)
- **FR-2.4**: Panic recovery handled by registry
- **FR-2.5**: Errors logged but don't block other listeners

### FR-3: Event Data Access

EventListener implementations SHALL have access to:

- **FR-3.1**: Event.ID - Unique event identifier
- **FR-3.2**: Event.Timestamp - Event occurrence time
- **FR-3.3**: Event.Type - Event category (string)
- **FR-3.4**: Event.Agent - AI agent platform (claude-code, cursor, etc.)
- **FR-3.5**: Event.Level - Severity level (Level type)
- **FR-3.6**: Event.SchemaVersion - Schema version for compatibility
- **FR-3.7**: Event.Data - Event-specific metadata (map[string]interface{})

### FR-4: Severity Level Filtering

EventListener implementations SHALL specify filtering:

- **FR-4.1**: MinLevel() returns minimum accepted severity
- **FR-4.2**: Events with level < MinLevel() are filtered
- **FR-4.3**: LevelInfo (0) - Informational events
- **FR-4.4**: LevelWarn (4) - Warning events
- **FR-4.5**: LevelError (8) - Error events
- **FR-4.6**: LevelCritical (12) - Critical system failures

### FR-5: Asynchronous Notification

EventListener implementations SHALL handle async calls:

- **FR-5.1**: OnEvent() called in separate goroutines
- **FR-5.2**: No ordering guarantees between events
- **FR-5.3**: Panics recovered and logged by registry
- **FR-5.4**: Errors logged but don't block other listeners
- **FR-5.5**: Must be thread-safe (concurrent OnEvent calls possible)

## Non-Functional Requirements

### NFR-1: Compatibility

- **NFR-1.1**: Go 1.21+ required
- **NFR-1.2**: No external dependencies (stdlib only)
- **NFR-1.3**: Backward compatible with internal/telemetry changes

### NFR-2: Performance

- **NFR-2.1**: Type re-export has zero runtime overhead
- **NFR-2.2**: EventListener filtering applied before goroutine spawn
- **NFR-2.3**: Listener execution doesn't block event recording

### NFR-3: Documentation

- **NFR-3.1**: Package godoc explains re-export purpose
- **NFR-3.2**: EventListener interface documented with example
- **NFR-3.3**: References P3 AGM Token Logger Plugin use case

## API Specification

### Type Re-exports

```go
package telemetry

import "github.com/vbonnet/engram/core/internal/telemetry"

// EventListener is the public interface for handling telemetry events.
// External modules can implement this interface to receive event notifications.
type EventListener = telemetry.EventListener

// Event represents a telemetry event with metadata.
type Event = telemetry.Event

// Level represents telemetry event severity levels.
type Level = telemetry.Level

// Severity level constants
const (
    LevelInfo     = telemetry.LevelInfo     // 0: Informational events
    LevelWarn     = telemetry.LevelWarn     // 4: Warning events
    LevelError    = telemetry.LevelError    // 8: Error events
    LevelCritical = telemetry.LevelCritical // 12: Critical failures
)
```

### EventListener Interface

```go
type EventListener interface {
    // OnEvent is called when an event is recorded (if level >= MinLevel).
    // Called asynchronously in a goroutine.
    // Panics are recovered and logged.
    // Errors are logged but don't block other listeners.
    OnEvent(event *Event) error

    // MinLevel returns the minimum severity level this listener accepts.
    // Events with level < MinLevel are filtered out before calling OnEvent.
    // Called once during registration (result is cached).
    MinLevel() Level
}
```

### Event Structure

```go
type Event struct {
    // Event ID (optional, for tracking)
    ID string `json:"id,omitempty"`

    // Event timestamp
    Timestamp time.Time `json:"timestamp"`

    // Event type (engram_loaded, plugin_executed, ecphory_query, etc.)
    Type string `json:"type"`

    // Agent platform (claude-code, cursor, windsurf, etc.)
    Agent string `json:"agent"`

    // Severity level (INFO, WARN, ERROR, CRITICAL)
    Level Level `json:"level"`

    // Schema version (for backward compatibility)
    SchemaVersion string `json:"schema_version,omitempty"`

    // Event-specific data
    Data map[string]interface{} `json:"data,omitempty"`
}
```

## Usage Patterns

### Pattern 1: Basic EventListener Implementation

```go
package mylogger

import (
    "fmt"
    "log"

    "github.com/vbonnet/engram/core/pkg/telemetry"
)

// TokenLogger logs AGM token usage events
type TokenLogger struct {
    outputPath string
}

func NewTokenLogger(path string) *TokenLogger {
    return &TokenLogger{outputPath: path}
}

// MinLevel filters to ERROR and above
func (l *TokenLogger) MinLevel() telemetry.Level {
    return telemetry.LevelError
}

// OnEvent processes telemetry events (thread-safe)
func (l *TokenLogger) OnEvent(event *telemetry.Event) error {
    if event.Type != "agm.token_usage" {
        return nil
    }

    tokens, ok := event.Data["tokens"].(float64)
    if !ok {
        return fmt.Errorf("invalid token data")
    }

    log.Printf("[%s] AGM token usage: %.0f tokens\n",
        event.Agent, tokens)

    return nil
}
```

### Pattern 2: Multi-Level Listener

```go
package monitor

import (
    "github.com/vbonnet/engram/core/pkg/telemetry"
)

// SystemMonitor tracks all event levels
type SystemMonitor struct {
    errorCount int
    warnCount  int
}

func (m *SystemMonitor) MinLevel() telemetry.Level {
    return telemetry.LevelInfo // Accept all levels
}

func (m *SystemMonitor) OnEvent(event *telemetry.Event) error {
    switch event.Level {
    case telemetry.LevelCritical, telemetry.LevelError:
        m.errorCount++
        // Send alert
    case telemetry.LevelWarn:
        m.warnCount++
        // Log warning
    case telemetry.LevelInfo:
        // Log info
    }
    return nil
}
```

### Pattern 3: Event Type Filtering

```go
package analytics

import (
    "github.com/vbonnet/engram/core/pkg/telemetry"
)

// PluginAnalytics tracks plugin usage
type PluginAnalytics struct {
    usageCounts map[string]int
}

func (a *PluginAnalytics) MinLevel() telemetry.Level {
    return telemetry.LevelInfo
}

func (a *PluginAnalytics) OnEvent(event *telemetry.Event) error {
    // Filter by event type
    if event.Type != "plugin.executed" {
        return nil
    }

    pluginName, ok := event.Data["plugin"].(string)
    if !ok {
        return nil
    }

    a.usageCounts[pluginName]++
    return nil
}
```

### Pattern 4: Registration with Collector

```go
package main

import (
    "github.com/vbonnet/engram/core/internal/telemetry"
    pkgtelemetry "github.com/vbonnet/engram/core/pkg/telemetry"
)

func main() {
    // Create collector (internal package)
    collector, err := telemetry.NewCollector(true, "~/.engram/telemetry.jsonl")
    if err != nil {
        panic(err)
    }
    defer collector.Close()

    // Create listener (public API)
    listener := &TokenLogger{outputPath: "/var/log/agm-tokens.log"}

    // Register listener with collector
    collector.AddListener(listener)
}
```

## Common Event Types

### Core Events

- `engram_loaded` - Engram pattern loaded
- `plugin_executed` - Plugin command executed
- `ecphory_query` - Ecphory retrieval operation
- `reflection_saved` - Reflection session saved
- `config_loaded` - Configuration initialized
- `eventbus_publish` - EventBus event published
- `eventbus_response` - EventBus response received

### Telemetry & Benchmarking Events

- `ecphory_audit_completed` - Ecphory correctness audit
- `persona_review_completed` - Multi-persona review
- `phase_transition_completed` - Wayfinder phase transition
- `agent_launch` - AI agent platform launch

## Event Data Examples

### Plugin Execution Event

```json
{
  "id": "evt-123",
  "timestamp": "2025-11-26T14:32:10Z",
  "type": "plugin.executed",
  "agent": "claude-code",
  "level": 0,
  "schema_version": "1.0.0",
  "data": {
    "plugin": "wayfinder",
    "command": "next",
    "duration_ms": 1250,
    "phase": "S2"
  }
}
```

### Ecphory Audit Event

```json
{
  "id": "evt-124",
  "timestamp": "2025-11-26T14:35:22Z",
  "type": "ecphory_audit_completed",
  "agent": "engram",
  "level": 0,
  "schema_version": "1.0.0",
  "data": {
    "session_id": "sess-abc",
    "total_retrievals": 25,
    "appropriate_count": 24,
    "inappropriate_count": 1,
    "correctness_score": 0.96,
    "audit_duration_ms": 450
  }
}
```

## Constraints and Assumptions

### Constraints

- **C-1**: Requires access to internal/telemetry for registration
- **C-2**: EventListener must be thread-safe (concurrent calls)
- **C-3**: No guarantee of event ordering
- **C-4**: Panics and errors are isolated per listener

### Assumptions

- **A-1**: External modules import pkg/telemetry, not internal/telemetry
- **A-2**: Collector created in main application (not external module)
- **A-3**: Listeners registered during application initialization
- **A-4**: Event.Data schema matches event type conventions

## Error Handling

### Error Categories

1. **Listener Panics**: Recovered by registry, logged, other listeners continue
2. **Listener Errors**: Logged by registry, other listeners continue
3. **Data Type Assertions**: Listener responsibility to handle type mismatches

### Best Practices

```go
func (l *MyListener) OnEvent(event *telemetry.Event) error {
    // Type assertions with ok-checks
    value, ok := event.Data["key"].(string)
    if !ok {
        return fmt.Errorf("unexpected data type for key")
    }

    // Handle errors gracefully
    if err := l.process(value); err != nil {
        return fmt.Errorf("processing failed: %w", err)
    }

    return nil
}
```

## Testing Requirements

### Unit Tests

- **T-1**: EventListener implementation compiles
- **T-2**: MinLevel() returns expected severity
- **T-3**: OnEvent() processes events correctly
- **T-4**: Thread safety under concurrent calls

### Integration Tests

- **T-5**: Listener receives events from collector
- **T-6**: Level filtering works correctly
- **T-7**: Panics don't crash application
- **T-8**: Errors logged but don't block other listeners

## Dependencies

- `github.com/vbonnet/engram/core/internal/telemetry` - Internal implementation
- Standard library only (no external dependencies)

## Use Cases

### P3 AGM Token Logger Plugin

External plugin that logs AGM API token usage for billing and monitoring:

```go
// ai-tools/plugins/agm-token-logger/listener.go
package main

import (
    "github.com/vbonnet/engram/core/pkg/telemetry"
)

type AGMTokenLogger struct {
    billing BillingService
}

func (l *AGMTokenLogger) MinLevel() telemetry.Level {
    return telemetry.LevelInfo
}

func (l *AGMTokenLogger) OnEvent(event *telemetry.Event) error {
    if event.Type != "agm.token_usage" {
        return nil
    }

    tokens := event.Data["tokens"].(float64)
    model := event.Data["model"].(string)

    return l.billing.RecordUsage(event.Agent, model, int(tokens))
}
```

## Future Considerations

- **F-1**: Event schema versioning and compatibility
- **F-2**: Structured event types (compile-time safety)
- **F-3**: Event filtering by type patterns
- **F-4**: Listener metrics (calls, errors, latency)
- **F-5**: Backpressure mechanisms for slow listeners
- **F-6**: Event batching for high-throughput scenarios
