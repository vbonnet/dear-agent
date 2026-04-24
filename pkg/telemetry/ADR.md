# Telemetry Public API - Architectural Decision Records

## ADR-001: Public API via Type Re-exports

**Status**: Accepted

**Context**:
External modules (ai-tools plugins, integrations) need to implement telemetry event listeners to receive notifications about Engram events. However, Go's internal package visibility rules prevent external modules from importing `internal/telemetry` directly.

The P3 AGM Token Logger Plugin is the primary use case: it needs to receive `agm.token_usage` events from Engram core to log API token consumption for billing purposes.

**Decision**:
Create a public API package (`pkg/telemetry`) that re-exports essential types from `internal/telemetry` using type aliases:

```go
// pkg/telemetry/telemetry.go
package telemetry

import "github.com/vbonnet/engram/core/internal/telemetry"

type EventListener = telemetry.EventListener
type Event = telemetry.Event
type Level = telemetry.Level

const (
    LevelInfo     = telemetry.LevelInfo
    LevelWarn     = telemetry.LevelWarn
    LevelError    = telemetry.LevelError
    LevelCritical = telemetry.LevelCritical
)
```

**Rationale**:
- **Type aliases have zero runtime overhead**: Go compiler resolves aliases at compile time
- **Full type compatibility**: External EventListener implementations work seamlessly with internal Collector
- **Minimal API surface**: Only export types needed for EventListener implementation
- **Internal implementation hiding**: External modules cannot access Collector or internal details
- **Backward compatibility**: Public API can remain stable while internal implementation changes

**Consequences**:
- **Positive**:
  - External modules can implement EventListener without internal package access
  - Zero performance overhead (type aliases are compile-time only)
  - Clean separation between public API and internal implementation
  - Stable public API enables external plugin ecosystem
  - Versioning flexibility (public API versioned separately from internal)

- **Negative**:
  - Requires maintaining two packages (pkg/telemetry and internal/telemetry)
  - Breaking changes to internal types may require public API updates
  - Documentation must be duplicated (godoc in pkg/telemetry)

- **Mitigation**:
  - Minimize public API surface (only EventListener types)
  - Comprehensive documentation in pkg/telemetry
  - Semantic versioning for public API (major version for breaking changes)

**Alternatives Considered**:

1. **Wrapper types instead of type aliases**:
   ```go
   type EventListener struct {
       impl telemetry.EventListener
   }
   ```
   - Rejected: Requires conversion functions, adds runtime overhead
   - Rejected: More complex API (wrapper methods needed)

2. **Make internal/telemetry public**:
   - Rejected: Exposes internal implementation details (Collector, Registry, etc.)
   - Rejected: External modules could create multiple Collectors (breaks singleton)
   - Rejected: Harder to maintain backward compatibility

3. **No public API, plugins use reflection**:
   - Rejected: Runtime overhead and complexity
   - Rejected: No compile-time type safety
   - Rejected: Fragile (internal changes break plugins)

4. **Plugin system with RPC/IPC**:
   - Rejected: Significant complexity (serialization, networking, etc.)
   - Rejected: Performance overhead (cross-process communication)
   - Rejected: Overkill for simple event notification

**Related Decisions**:
- ADR-007 (core/docs/adr/telemetry-system.md): Overall telemetry system architecture
- P3 AGM Token Logger Plugin: Primary use case for public API

---

## ADR-002: Listener Registration via Main Application Only

**Status**: Accepted

**Context**:
External modules implementing EventListener need to register with the Collector to receive events. Where should listener registration happen: in external modules or main application?

**Decision**:
Listener registration MUST occur in the main application (which has access to both `internal/telemetry` and `pkg/telemetry`). External modules only implement EventListener interface, they cannot self-register.

**Registration Pattern**:
```go
// main.go (Engram core - has internal access)
import (
    "github.com/vbonnet/engram/core/internal/telemetry"
    "ai-tools/plugins/agm-token-logger"
)

func main() {
    // Create Collector (internal package)
    collector, _ := telemetry.NewCollector(true, "~/.engram/telemetry.jsonl")
    defer collector.Close()

    // Create external listener (public API)
    listener := agmtokenlogger.NewListener()

    // Main application registers listener
    collector.AddListener(listener)
}
```

**Rationale**:
- **Singleton Collector**: Main application controls telemetry lifecycle (single Collector instance)
- **Configuration control**: Main application reads config, decides whether telemetry is enabled
- **Dependency inversion**: External modules depend on stable public API, not internal Collector
- **Security**: External modules cannot access JSONL files or bypass telemetry settings
- **Simplicity**: External modules are passive (implement interface, nothing else)

**Consequences**:
- **Positive**:
  - External modules cannot create duplicate Collectors
  - External modules have minimal dependencies (pkg/telemetry only)
  - Main application has full control over telemetry lifecycle
  - Configuration centralized in main application

- **Negative**:
  - Main application must know about all external listeners (explicit registration)
  - Plugin discovery requires main application changes (not fully dynamic)

- **Mitigation**:
  - Plugin loader can discover and register listeners automatically
  - Configuration file can specify which listeners to load

**Alternatives Considered**:

1. **Self-registration in external modules**:
   ```go
   func init() {
       telemetry.RegisterListener(&MyListener{})
   }
   ```
   - Rejected: Requires global singleton Collector (poor design)
   - Rejected: Configuration control lost (listeners auto-register)
   - Rejected: Testing harder (global state)

2. **Registry pattern with dependency injection**:
   ```go
   registry := telemetry.NewRegistry()
   listener := agmtokenlogger.NewListener(registry)
   ```
   - Rejected: More complex API surface
   - Rejected: External modules need Registry type (expands public API)

3. **Plugin discovery with auto-registration**:
   - Deferred: Can be implemented on top of explicit registration
   - Future: Plugin loader scans plugins for EventListener implementations

**Related Decisions**:
- ADR-001: Public API provides EventListener types
- Plugin system design (future): May add auto-discovery

---

## ADR-003: Minimal Public API Surface

**Status**: Accepted

**Context**:
What types and functions should `pkg/telemetry` export? Should it include helper functions, configuration types, or just EventListener essentials?

**Decision**:
Export ONLY the minimum types required to implement EventListener:
- EventListener interface (type alias)
- Event struct (type alias)
- Level type (type alias)
- Level constants (LevelInfo, LevelWarn, LevelError, LevelCritical)

**NOT exported**:
- Collector (event collection and storage)
- ListenerRegistry (listener management)
- Configuration types (TelemetryConfig, etc.)
- Helper functions (GenerateSessionSalt, etc.)
- Internal utilities

**Rationale**:
- **Simplicity**: Smaller API is easier to learn and maintain
- **Stability**: Fewer exported types means fewer breaking changes
- **Encapsulation**: Internal implementation details remain hidden
- **Security**: External modules cannot create Collectors or access event logs
- **Versioning**: Minimal API can remain stable across major versions

**Consequences**:
- **Positive**:
  - Simple, focused API for external modules
  - Easy to version (few types, stable signatures)
  - Clear separation of concerns (implement vs. manage)
  - Reduced attack surface (no Collector access)

- **Negative**:
  - External modules cannot create Collectors (must use main app)
  - No helper functions available (e.g., event filtering utilities)
  - External modules duplicate code if needed (e.g., type assertions)

- **Mitigation**:
  - Future: Add optional helper package (pkg/telemetry/helpers)
  - Documentation includes common patterns and examples

**API Evolution Path**:

```
v1.0.0 (Current)
├── EventListener interface
├── Event struct
├── Level type
└── Level constants

v1.1.0 (Future - Backward Compatible)
├── EventListener interface
├── Event struct
├── Level type
├── Level constants
└── pkg/telemetry/helpers (NEW)
    ├── func FilterByType(event *Event, types ...string) bool
    └── func ExtractString(data map[string]interface{}, key string) (string, error)

v2.0.0 (Future - Breaking Change)
├── EventListener interface (NEW METHOD)
│   ├── OnEvent(event *Event) error
│   ├── MinLevel() Level
│   └── OnBatch(events []*Event) error (NEW)
└── ...
```

**Alternatives Considered**:

1. **Export Collector and Registry**:
   - Rejected: Breaks singleton pattern (multiple Collectors)
   - Rejected: Exposes internal implementation (file format, etc.)
   - Rejected: Security risk (external modules access event logs)

2. **Export helper functions**:
   ```go
   func pkg/telemetry.FilterEvents(events []*Event, filter func(*Event) bool) []*Event
   ```
   - Deferred: Can add in v1.1.0 if needed
   - Current: External modules implement their own helpers

3. **Export configuration types**:
   ```go
   type TelemetryConfig struct { ... }
   ```
   - Rejected: External modules don't configure telemetry
   - Rejected: Configuration is main application responsibility

**Related Decisions**:
- ADR-001: Type re-exports strategy
- ADR-002: Registration via main application

---

## ADR-004: Thread Safety Requirements for EventListener

**Status**: Accepted

**Context**:
EventListener.OnEvent() is called asynchronously in goroutines. What thread safety guarantees should the interface require?

**Decision**:
EventListener implementations MUST be thread-safe:
- OnEvent() may be called concurrently from multiple goroutines
- Implementations must use locks if they have mutable state
- Event data is read-only (should not be modified)
- MinLevel() called once during registration (result cached)

**Interface Contract**:
```go
type EventListener interface {
    // OnEvent is called asynchronously in a goroutine.
    // MUST be thread-safe (may be called concurrently).
    // Event data is read-only (do not modify).
    // Panics are recovered and logged.
    // Errors are logged but don't block other listeners.
    OnEvent(event *Event) error

    // MinLevel returns the minimum severity level this listener accepts.
    // Called once during registration (result is cached).
    // MUST return stable value (no side effects).
    MinLevel() Level
}
```

**Rationale**:
- **Performance**: Async notification enables non-blocking event recording
- **Isolation**: Concurrent execution prevents slow listeners from blocking others
- **Consistency**: Thread-safe requirement is standard for async callbacks
- **Safety**: Event read-only prevents data races

**Consequences**:
- **Positive**:
  - High throughput (parallel listener execution)
  - Isolation (one listener can't block others)
  - Simple Registry implementation (no ordering requirements)

- **Negative**:
  - External modules must implement thread safety (more complex)
  - Debugging harder (concurrent execution, no ordering)

- **Mitigation**:
  - Documentation includes thread-safe examples
  - Common patterns documented (mutex, channels, immutable state)

**Thread-Safe Listener Examples**:

**Pattern 1: Mutex for mutable state**:
```go
type CountingListener struct {
    mu    sync.Mutex
    count int
}

func (l *CountingListener) OnEvent(event *Event) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.count++
    return nil
}
```

**Pattern 2: Channel for async processing**:
```go
type QueuedListener struct {
    queue chan *Event
}

func (l *QueuedListener) OnEvent(event *Event) error {
    select {
    case l.queue <- event:
        return nil
    default:
        return fmt.Errorf("queue full")
    }
}
```

**Pattern 3: Immutable state (no locks needed)**:
```go
type StatelessListener struct {
    logger *log.Logger // Logger is thread-safe
}

func (l *StatelessListener) OnEvent(event *Event) error {
    l.logger.Printf("Event: %s", event.Type)
    return nil
}
```

**Alternatives Considered**:

1. **Sequential notification (single goroutine)**:
   ```go
   for _, listener := range listeners {
       listener.OnEvent(event)  // Sequential, no concurrency
   }
   ```
   - Rejected: Slow listeners block fast listeners
   - Rejected: Poor performance (no parallelism)

2. **Listener specifies concurrency model**:
   ```go
   type EventListener interface {
       IsConcurrentSafe() bool
   }
   ```
   - Rejected: More complex Registry implementation
   - Rejected: Two execution paths (sequential vs parallel)

3. **Copy Event for each listener**:
   ```go
   eventCopy := *event  // Defensive copy
   go listener.OnEvent(&eventCopy)
   ```
   - Rejected: Performance overhead (allocations)
   - Rejected: Deep copy needed for Data map (expensive)
   - Current: Document read-only requirement

**Related Decisions**:
- ADR-001: Type re-exports enable interface implementation
- internal/telemetry: Registry.Notify() spawns goroutines

---

## ADR-005: Documentation Strategy for Public API

**Status**: Accepted

**Context**:
External module developers need comprehensive documentation to implement EventListener correctly. Where should documentation live: pkg/telemetry godoc, examples, wiki, or external docs?

**Decision**:
Multi-tier documentation strategy:

**Tier 1: Package godoc (pkg/telemetry/telemetry.go)**:
- Purpose statement (why public API exists)
- Re-export explanation (why type aliases)
- Reference to P3 AGM Token Logger Plugin
- Link to SPEC.md for details

**Tier 2: SPEC.md (pkg/telemetry/SPEC.md)**:
- Functional requirements (FR-1 to FR-5)
- API specification (types, methods)
- Usage patterns (4+ examples)
- Common event types
- Event data examples (JSON)

**Tier 3: ARCHITECTURE.md (pkg/telemetry/ARCHITECTURE.md)**:
- System overview (public API layer)
- Integration patterns (plugin-based, library-based)
- Thread safety requirements
- Performance characteristics
- Testing strategy

**Tier 4: ADR.md (pkg/telemetry/ADR.md)**:
- Architectural decisions (this file)
- Rationale for design choices
- Alternatives considered
- Trade-offs and consequences

**Rationale**:
- **Godoc**: First place developers look (must be clear)
- **SPEC.md**: Detailed reference for implementation
- **ARCHITECTURE.md**: Understanding system design
- **ADR.md**: Historical context and decision rationale

**Consequences**:
- **Positive**:
  - Multiple entry points for different audiences
  - Progressive disclosure (brief → detailed)
  - Examples in SPEC.md reduce learning curve

- **Negative**:
  - Documentation maintenance burden (4 files)
  - Potential inconsistencies between files

- **Mitigation**:
  - Tests validate examples in SPEC.md
  - ADR references SPEC.md (single source of truth for API)

**Documentation Examples Required**:

**SPEC.md Examples** (4 patterns minimum):
1. Basic EventListener implementation
2. Multi-level listener (all severity levels)
3. Event type filtering
4. Registration with Collector

**ARCHITECTURE.md Examples** (3 patterns):
1. Plugin-based listener (external module)
2. Library-based listener (shared utilities)
3. In-process extension (main application)

**Godoc Example** (simple, inline):
```go
// EventListener is the public interface for handling telemetry events.
// External modules can implement this interface to receive event notifications.
//
// Example:
//
//  type MyListener struct{}
//
//  func (l *MyListener) MinLevel() Level {
//      return LevelWarn  // Only WARN, ERROR, CRITICAL
//  }
//
//  func (l *MyListener) OnEvent(event *Event) error {
//      log.Printf("Event: %s", event.Type)
//      return nil
//  }
type EventListener = telemetry.EventListener
```

**Alternatives Considered**:

1. **Godoc only** (no SPEC.md or ARCHITECTURE.md):
   - Rejected: Too brief for comprehensive understanding
   - Rejected: No place for detailed examples

2. **External wiki or docs site**:
   - Rejected: Harder to keep in sync with code
   - Rejected: Not co-located with package

3. **Examples in separate examples/ directory**:
   - Considered: Can add in future
   - Current: Examples in SPEC.md (easier to maintain)

**Related Decisions**:
- ADR-001: Public API design
- ADR-004: Thread safety requirements (documented in SPEC.md)

---

## Implementation Status

### Implemented (v1.0.0)

- ✅ Type aliases for EventListener, Event, Level (ADR-001)
- ✅ Minimal public API surface (ADR-003)
- ✅ Registration via main application pattern (ADR-002)
- ✅ Thread safety requirements documented (ADR-004)
- ✅ Multi-tier documentation (ADR-005)
- ✅ SPEC.md with 4+ usage patterns
- ✅ ARCHITECTURE.md with integration patterns
- ✅ ADR.md with decision records (this file)

### Planned (Future)

- ⏳ Helper package (pkg/telemetry/helpers) in v1.1.0
- ⏳ Plugin auto-discovery and registration
- ⏳ Event batching for high-throughput listeners
- ⏳ Structured event types (compile-time safety)

## References

- **ADR-007** (core/docs/adr/telemetry-system.md): Overall telemetry system design
- **P3 AGM Token Logger Plugin**: Primary use case for public API
- **internal/telemetry**: Implementation of types re-exported by pkg/telemetry
