# Telemetry Public API - Architecture

## System Overview

The `pkg/telemetry` package serves as a thin public API layer that re-exports types from `internal/telemetry`, enabling external modules to implement telemetry event listeners without violating Go's internal package visibility rules.

This architecture supports the P3 AGM Token Logger Plugin and future telemetry integrations while maintaining clean separation between internal implementation and public API.

## Architectural Principles

1. **Minimal Surface Area**: Re-export only essential types for EventListener implementation
2. **Zero Runtime Overhead**: Type aliases have no performance cost
3. **Internal Implementation Hiding**: External modules cannot access internal/telemetry directly
4. **Backward Compatibility**: Public API stable, internal changes don't break external modules
5. **Documentation First**: Public API comprehensively documented for external consumers

## Component Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    External Modules                          │
│              (ai-tools, plugins, integrations)               │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        │ import "pkg/telemetry"
                        │
                        ▼
┌──────────────────────────────────────────────────────────────┐
│                  pkg/telemetry (Public API)                  │
│                                                              │
│  - EventListener interface (type alias)                     │
│  - Event struct (type alias)                                │
│  - Level type (type alias)                                  │
│  - Level constants (re-export)                              │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        │ type alias to internal/telemetry
                        │
                        ▼
┌──────────────────────────────────────────────────────────────┐
│              internal/telemetry (Implementation)             │
│                                                              │
│  - Collector (event collection, storage)                    │
│  - ListenerRegistry (listener management)                   │
│  - EventListener interface (actual definition)              │
│  - Event struct (actual definition)                         │
│  - Level type (actual definition)                           │
└──────────────────────────────────────────────────────────────┘
```

## Component Details

### 1. Public API Layer (pkg/telemetry)

**Responsibility**: Provide stable, documented public interface for external modules

**Architecture**:
```go
// pkg/telemetry/telemetry.go
package telemetry

import "github.com/vbonnet/engram/core/internal/telemetry"

// Type aliases (zero runtime overhead)
type EventListener = telemetry.EventListener
type Event = telemetry.Event
type Level = telemetry.Level

// Constant re-exports
const (
    LevelInfo     = telemetry.LevelInfo
    LevelWarn     = telemetry.LevelWarn
    LevelError    = telemetry.LevelError
    LevelCritical = telemetry.LevelCritical
)
```

**Design Decisions**:
- Type aliases (`=`) instead of new types (zero overhead, full compatibility)
- Re-export constants for convenience
- No functions exported (registration happens via internal package)
- Minimal godoc with clear purpose statement

**Versioning Strategy**:
- Public API versioned separately from internal implementation
- Breaking changes to public API require major version bump
- Internal changes transparent to external modules

### 2. Internal Implementation (internal/telemetry)

**Responsibility**: Implement telemetry collection, storage, and listener notification

See `internal/telemetry/ARCHITECTURE.md` for full internal architecture details.

**Key Components**:
- **Collector**: Event collection and JSONL storage
- **ListenerRegistry**: Thread-safe listener management and async notification
- **EventListener**: Interface for event handlers
- **Event**: Event data structure
- **Level**: Severity level enumeration

### 3. External Module Integration

**Responsibility**: External modules implement EventListener and register with Collector

**Integration Pattern**:
```
External Module
    │
    ├─→ Import pkg/telemetry
    │   └─→ Implement EventListener interface
    │
    └─→ Register with Collector (via main application)
        └─→ Receive async event notifications
```

**Example Flow**:
```
1. Main application creates Collector (internal/telemetry)
2. External module implements EventListener (pkg/telemetry)
3. Main application calls collector.AddListener(externalListener)
4. Registry stores listener reference
5. Events recorded → Registry notifies listener async
```

## Data Flow

### Event Notification Flow

```
Event Recorded
    │
    ▼
Collector.Record()
    │
    ├─→ Write to JSONL file
    │
    └─→ Registry.Notify(event)
            │
            ├─→ Filter by level (event.Level >= listener.MinLevel())
            │
            └─→ For each matching listener:
                    │
                    └─→ Spawn goroutine → listener.OnEvent(event)
                            │
                            ├─→ Success → return nil
                            ├─→ Error → log error, continue
                            └─→ Panic → recover, log, continue
```

### Type Flow (Compile-Time)

```
External Module Code:
    import "github.com/vbonnet/engram/core/pkg/telemetry"

    type MyListener struct{}

    func (l *MyListener) OnEvent(event *telemetry.Event) error {
        // event is *internal/telemetry.Event (via type alias)
    }

    func (l *MyListener) MinLevel() telemetry.Level {
        // Level is internal/telemetry.Level (via type alias)
        return telemetry.LevelInfo
    }

Go Compiler:
    telemetry.Event → internal/telemetry.Event (type alias resolution)
    telemetry.Level → internal/telemetry.Level (type alias resolution)
    telemetry.EventListener → internal/telemetry.EventListener (interface)
```

## Integration Patterns

### Pattern 1: Plugin-Based Listener

**Use Case**: External plugin implements EventListener

```
ai-tools/plugins/agm-token-logger/
    ├── main.go              # Plugin entry point
    ├── listener.go          # EventListener implementation
    └── go.mod               # Depends on pkg/telemetry

main.go (Engram core):
    import "ai-tools/plugins/agm-token-logger"

    collector, _ := telemetry.NewCollector(true, path)
    plugin := agmtokenlogger.NewListener()
    collector.AddListener(plugin)
```

**Benefits**:
- Plugin compiled separately from Engram core
- No internal package access violations
- Plugin can be versioned independently

### Pattern 2: Library-Based Listener

**Use Case**: Shared library provides EventListener implementations

```
ai-tools/lib/telemetry-utils/
    ├── file_logger.go       # File-based EventListener
    ├── metrics_logger.go    # Metrics EventListener
    └── go.mod               # Depends on pkg/telemetry

main.go (Engram core):
    import "ai-tools/lib/telemetry-utils"

    collector, _ := telemetry.NewCollector(true, path)
    fileLogger := telemetryutils.NewFileLogger("/var/log/engram.log")
    metricsLogger := telemetryutils.NewMetricsLogger()

    collector.AddListener(fileLogger)
    collector.AddListener(metricsLogger)
```

**Benefits**:
- Reusable EventListener implementations
- External modules depend only on stable public API
- Testing independent of Engram core

### Pattern 3: In-Process Extension

**Use Case**: Main application extends telemetry with custom listeners

```
main.go (Engram core):
    import (
        "github.com/vbonnet/engram/core/internal/telemetry"
        pkgtelemetry "github.com/vbonnet/engram/core/pkg/telemetry"
    )

    type CustomListener struct{}

    func (l *CustomListener) OnEvent(event *pkgtelemetry.Event) error {
        // Custom processing
    }

    func (l *CustomListener) MinLevel() pkgtelemetry.Level {
        return pkgtelemetry.LevelWarn
    }

    collector, _ := telemetry.NewCollector(true, path)
    collector.AddListener(&CustomListener{})
```

**Benefits**:
- No external dependencies
- Direct access to both internal and public APIs
- Ideal for core functionality

## Concurrency Model

### Thread Safety

**Public API (pkg/telemetry)**:
- Type aliases are compile-time constructs (no runtime state)
- No concurrency concerns in public API layer

**External EventListener Implementations**:
- **MUST be thread-safe**: OnEvent() called concurrently in goroutines
- **Read-only access**: Event data should not be modified
- **Local state**: Use locks if listener has mutable state

**Example Thread-Safe Listener**:
```go
type CountingListener struct {
    mu    sync.Mutex
    count int
}

func (l *CountingListener) OnEvent(event *telemetry.Event) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.count++
    return nil
}
```

### Async Notification

```
Registry.Notify(event)
    │
    └─→ For each listener (read lock held):
            │
            └─→ go func(listener) {
                    defer recover()  // Panic isolation
                    listener.OnEvent(event)
                }(listener)
```

**Properties**:
- Non-blocking: Notify() returns immediately
- No ordering guarantees between listeners
- Panic isolation: One listener panic doesn't affect others
- Error isolation: One listener error doesn't affect others

## API Stability Guarantees

### Stable (Won't Change)

- **EventListener interface**: OnEvent() and MinLevel() signatures
- **Event struct fields**: ID, Timestamp, Type, Agent, Level, SchemaVersion, Data
- **Level constants**: LevelInfo, LevelWarn, LevelError, LevelCritical values

### Extensible (Can Add)

- **Event.Data fields**: New event types can add new data fields
- **Event types**: New event type strings (backward compatible)
- **Level values**: New severity levels (e.g., LevelDebug)

### Internal (Can Change)

- **Collector implementation**: File format, rotation, etc.
- **Registry implementation**: Notification mechanism, filtering, etc.
- **Event schema version**: SchemaVersion field supports evolution

## Versioning Strategy

### Semantic Versioning

```
v1.0.0 (Current)
    - EventListener interface
    - Event struct
    - Level type and constants

v1.1.0 (Future - Backward Compatible)
    - Add new Event types
    - Add new Level constants
    - Add new Event.Data fields

v2.0.0 (Future - Breaking Change)
    - Change EventListener interface (e.g., add method)
    - Remove Event fields
    - Change Level constant values
```

### Deprecation Policy

- Deprecated APIs marked with `// Deprecated:` comment
- Deprecated APIs maintained for 2 minor versions
- Breaking changes only in major version bumps

## Error Handling Strategy

### Public API Layer

**No error handling** (type aliases only):
- No functions to fail
- Type resolution at compile time

### External EventListener Implementations

**Listener Responsibility**:
- Return errors from OnEvent() for non-fatal issues
- Panic for unrecoverable errors (recovered by registry)
- Handle type assertions gracefully

**Example Error Handling**:
```go
func (l *MyListener) OnEvent(event *telemetry.Event) error {
    // Type assertion with error return
    value, ok := event.Data["key"].(string)
    if !ok {
        return fmt.Errorf("invalid data type for key")
    }

    // Processing with error propagation
    if err := l.process(value); err != nil {
        return fmt.Errorf("processing failed: %w", err)
    }

    return nil
}
```

**Registry Behavior**:
- Errors logged via slog.Default().Error()
- Other listeners continue executing
- Event recording succeeds even if listeners fail

## Security Considerations

### Type Safety

- **Compile-time checking**: Type aliases enforce type safety
- **No reflection**: Direct type access, no runtime reflection
- **Interface segregation**: EventListener minimal, reduces attack surface

### External Module Isolation

- **Internal package barrier**: External modules cannot access internal/telemetry
- **API surface minimized**: Only EventListener types exported
- **No filesystem access**: External modules receive events, don't access JSONL files

### Event Data Validation

**Listener Responsibility**:
- Validate Event.Data field types
- Sanitize strings from Event.Data
- Avoid executing untrusted data from events

**Example Sanitization**:
```go
func (l *MyListener) OnEvent(event *telemetry.Event) error {
    pluginName, ok := event.Data["plugin"].(string)
    if !ok {
        return fmt.Errorf("invalid plugin name type")
    }

    // Sanitize plugin name (prevent path traversal)
    if strings.Contains(pluginName, "/") || strings.Contains(pluginName, "..") {
        return fmt.Errorf("invalid plugin name: %s", pluginName)
    }

    return l.logPlugin(pluginName)
}
```

## Performance Characteristics

### Type Alias Overhead

- **Zero runtime cost**: Type aliases resolved at compile time
- **No memory overhead**: Alias types are identical to underlying types
- **No indirection**: Direct access to internal types

### EventListener Notification

- **Async notification**: O(1) per listener (spawns goroutine)
- **Level filtering**: O(1) comparison before goroutine spawn
- **Parallel execution**: All listeners execute concurrently

### Event Data Access

- **Map access**: O(1) for Event.Data field lookup
- **Type assertion**: O(1) runtime type check
- **No serialization**: Event passed by pointer (no copying)

## Testing Strategy

### Public API Tests

**Type Alias Verification**:
```go
func TestTypeAliases(t *testing.T) {
    // Verify EventListener interface compatibility
    var listener EventListener = &testListener{}

    // Verify Event struct compatibility
    event := &Event{Type: "test"}

    // Verify Level type compatibility
    var level Level = LevelInfo
}
```

**External Module Simulation**:
```go
// Test external module implementing EventListener
type ExternalListener struct{}

func (l *ExternalListener) OnEvent(event *Event) error {
    return nil
}

func (l *ExternalListener) MinLevel() Level {
    return LevelInfo
}

func TestExternalListenerIntegration(t *testing.T) {
    // Verify external listener works with internal collector
}
```

### Integration Tests

**End-to-End Flow**:
1. Create Collector (internal/telemetry)
2. Create EventListener (pkg/telemetry types)
3. Register listener with collector
4. Record event
5. Verify listener receives event

## Deployment Considerations

### External Module Requirements

- **Go version**: 1.21+ (same as Engram core)
- **Import path**: `github.com/vbonnet/engram/core/pkg/telemetry`
- **Build constraints**: None (pure Go)

### Compatibility Matrix

| Engram Core Version | pkg/telemetry Version | EventListener API |
|---------------------|----------------------|-------------------|
| v0.1.0              | v1.0.0               | OnEvent, MinLevel |
| v0.2.0              | v1.0.0               | OnEvent, MinLevel |
| v1.0.0              | v1.0.0               | OnEvent, MinLevel |

## Future Architecture Enhancements

### Planned Improvements

1. **Structured Event Types**: Type-safe event data (compile-time checking)
2. **Event Batching**: Batch events for high-throughput listeners
3. **Backpressure**: Slow listener detection and circuit breaking
4. **Listener Metrics**: Track listener performance (latency, errors)
5. **Event Schema Registry**: Centralized event type definitions

### Extensibility Points

- **EventListener interface**: Can add methods in v2.0.0
- **Event struct**: SchemaVersion supports field evolution
- **Level type**: Can add new severity levels (backward compatible)

## Dependencies

### Public API Dependencies

- `github.com/vbonnet/engram/core/internal/telemetry` - Type definitions
- Standard library only (time package for Event.Timestamp)

### External Module Dependencies

- `github.com/vbonnet/engram/core/pkg/telemetry` - Public API
- Application-specific dependencies (logging, metrics, etc.)

## Related Documentation

- **internal/telemetry/ARCHITECTURE.md**: Internal telemetry implementation
- **ADR-007**: Telemetry system design decisions
- **P3 AGM Token Logger**: Primary use case for public API

## Design Rationale

### Why Type Aliases Instead of Wrapper Types?

**Type Aliases**:
```go
type EventListener = telemetry.EventListener // ✅ Zero overhead
```

**Wrapper Types** (NOT used):
```go
type EventListener telemetry.EventListener // ❌ New type, requires conversion
```

**Rationale**:
- Type aliases have zero runtime overhead
- Full compatibility with internal types (no conversions)
- External modules get exact same types as internal code
- Simpler to implement and maintain

### Why Minimal Public API?

**Only export EventListener types**:
- External modules only need to implement listeners
- Registration handled by main application (has internal access)
- Prevents external modules from creating Collectors (consistency)
- Reduces API surface area (easier to maintain)

### Why No Public Functions?

**No Collector constructor in public API**:
- Collector creation requires internal package access (intentional)
- Main application controls telemetry lifecycle
- External modules are passive (receive events only)
- Prevents multiple Collector instances (singleton pattern)

## Implementation Status

### Implemented (v1.0.0)

- ✅ Type aliases for EventListener, Event, Level
- ✅ Constant re-exports for severity levels
- ✅ Package documentation with examples
- ✅ Reference to P3 AGM Token Logger use case

### Planned (v1.1.0+)

- ⏳ Structured event types (compile-time safety)
- ⏳ Event filtering by type patterns
- ⏳ Listener performance metrics
- ⏳ Backpressure mechanisms
