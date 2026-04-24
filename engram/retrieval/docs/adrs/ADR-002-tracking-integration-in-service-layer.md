# ADR-002: Tracking Integration in Service Layer

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Integration of access tracking with retrieval system

---

## Context

Engram access tracking (recording last_accessed and access_count in frontmatter) needs to be
integrated somewhere in the retrieval pipeline. Three potential integration points:

1. **Ecphory Layer**: Ecphory emits events via EventBus, tracking subscribes
2. **Retrieval Layer**: Service records access for each retrieved engram
3. **Consumer Layer**: CLI/API manually call tracking after search

**Requirements**:
- Track every successful retrieval (not index-only metadata access)
- Update engram frontmatter asynchronously (don't block search)
- Handle tracking failures gracefully (non-critical operation)

---

## Decision

We will integrate tracking in the **Retrieval Service layer**, recording access for each
successfully parsed engram returned in search results.

**Implementation**:
```go
type Service struct {
    parser  *engram.Parser
    tracker *tracking.Tracker  // Owned by Service
}

func NewService() *Service {
    updater := tracking.NewMetadataUpdater()
    tracker := tracking.NewTracker(updater)
    return &Service{
        parser:  engram.NewParser(),
        tracker: tracker,
    }
}

func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
    // ... search logic ...
    for _, path := range resultPaths {
        eg, err := s.parser.Parse(path)
        if err != nil {
            continue // Skip unparseable
        }
        results = append(results, &SearchResult{Path: path, Engram: eg})

        // Track access for this engram
        s.tracker.RecordAccess(path, time.Now())
    }
    return results, nil
}

func (s *Service) Close() error {
    // Flush pending updates
    return s.tracker.Flush()
}
```

**Characteristics**:
- Tracking happens after successful parse (only count retrieved engrams)
- Updates are batched in tracker (async write on Close)
- Service owns tracker lifecycle (create in NewService, flush in Close)

---

## Consequences

### Positive

**Separation of Concerns**:
- Ecphory remains focused on search algorithm (no telemetry logic)
- Tracking is transparent to consumers (automatic on Search)
- Service layer is natural boundary (knows about file paths and lifecycle)

**Accurate Tracking**:
- Only counts successfully parsed engrams (not failed reads)
- Tracks actual retrievals (not index-only metadata scans)
- One access record per Search call (clear semantics)

**Simple API**:
- Consumers don't need to call tracking manually
- No EventBus plumbing in consumer code
- Automatic lifecycle management (Close flushes updates)

**Testability**:
- Retrieval tests can verify tracking calls
- Ecphory tests remain isolated (no tracking dependencies)
- Tracking can be mocked at Service layer if needed

### Negative

**Service Owns Non-Search Responsibility**:
- Service now handles two concerns: search + tracking
- Mixing core function (search) with side effect (telemetry)
- This is **acceptable** because:
  - Tracking is lightweight (queue operation, not blocking)
  - Close() provides explicit flush point (no hidden async work)
  - Alternative (consumer tracking) would distribute responsibility

**Close() Required for Flush**:
- Consumers must call Close() or updates may be lost
- Forgot Close() → tracking updates remain in memory
- This is **acceptable** because:
  - Close() is standard lifecycle pattern (like database Close)
  - Best-effort semantics (tracking is non-critical)
  - CLI defers Close() in main (safety net)

**No EventBus Decoupling**:
- Service directly depends on tracking package
- Can't swap tracking implementation without changing Service
- This is **acceptable** because:
  - Tracking is internal package (not external dependency)
  - No foreseeable need for alternate tracking systems
  - Can add EventBus later if multi-consumer use case emerges

---

## Alternatives Considered

### Alternative 1: Ecphory Emits Events, Tracking Subscribes

**Approach**:
```go
// In ecphory
func (e *Ecphory) Query(ctx, query, tags, agent) ([]Result, error) {
    results := e.rank(...)
    for _, r := range results {
        e.eventBus.Publish(ctx, &Event{
            Topic: "engram.accessed",
            Data:  map[string]interface{}{"path": r.Path},
        })
    }
    return results, nil
}

// In retrieval
func NewService() *Service {
    eventBus := eventbus.New()
    tracker := tracking.NewTracker(updater)
    eventBus.Subscribe("engram.accessed", tracker.HandleEvent)

    ecphory := ecphory.NewEcphory(engramPath, tokenBudget)
    ecphory.SetEventBus(eventBus)
    return &Service{ecphory: ecphory}
}
```

**Rejected Because**:
- **Complexity**: Requires EventBus plumbing in ecphory, retrieval, and tracking
- **Coupling**: Ecphory now depends on EventBus (cross-cutting concern)
- **Premature**: No other ecphory event consumers (YAGNI)
- **Timing**: Ecphory doesn't parse engrams (tracks ranking, not retrieval)

### Alternative 2: Consumer Manual Tracking

**Approach**:
```go
// In CLI
service := retrieval.NewService()
results, _ := service.Search(ctx, opts)

tracker := tracking.NewTracker(updater)
for _, r := range results {
    tracker.RecordAccess(r.Path, time.Now())
}
tracker.Flush()
```

**Rejected Because**:
- **Distributed Responsibility**: Every consumer must remember to track
- **Error-Prone**: Easy to forget tracking (silent degradation)
- **Duplication**: Tracking code repeated in CLI, API, SDK
- **Inconsistency**: Different consumers may track differently

### Alternative 3: Tracking as Optional Callback

**Approach**:
```go
type Service struct {
    onAccess func(path string) // Optional callback
}

func (s *Service) SetTracker(fn func(path string)) {
    s.onAccess = fn
}

func (s *Service) Search(...) ([]*SearchResult, error) {
    // ... search ...
    if s.onAccess != nil {
        s.onAccess(path)
    }
}
```

**Rejected Because**:
- **Indirection**: Adds complexity without clear benefit
- **Lifecycle**: Who manages tracker lifetime? (Service or consumer?)
- **Testing**: Harder to verify tracking (callback vs concrete tracker)
- **Not Flexible**: If we need callbacks, use EventBus (more powerful)

---

## Implementation Notes

**Tracking Code** (lines 169-170 in `retrieval.go`):
```go
// Track access for this engram
s.tracker.RecordAccess(path, time.Now())
```

**Flush Code** (lines 177-182 in `retrieval.go`):
```go
func (s *Service) Close() error {
    if err := s.tracker.Flush(); err != nil {
        log.Printf("retrieval: failed to flush tracking updates: %v", err)
        // Don't return error - this is best-effort
    }
    return nil
}
```

**Best-Effort Semantics**:
- Flush errors are logged, not returned
- Service.Close() always returns nil
- Tracking is non-critical (doesn't block cleanup)

**CLI Usage Pattern**:
```go
func main() {
    service := retrieval.NewService()
    defer service.Close() // Ensures flush on exit

    results, err := service.Search(ctx, opts)
    // Use results...
}
```

---

## Related Decisions

- **ADR-001**: Per-Search Index Building (affects Service lifecycle)
- **ADR-003**: API Ranking Fallback Strategy (tracking only counts successful retrievals)

---

## Future Considerations

If additional telemetry consumers emerge (e.g., analytics, audit logs), migrate to EventBus:

```go
type Service struct {
    eventBus EventBus
}

func (s *Service) Search(...) {
    // ... search ...
    s.eventBus.Publish(ctx, &Event{
        Topic: "retrieval.engram_accessed",
        Data:  map[string]interface{}{"path": path, "timestamp": time.Now()},
    })
}

// Tracking subscribes to events
eventBus.Subscribe("retrieval.engram_accessed", tracker.HandleEvent)
```

This preserves separation of concerns while enabling multiple consumers.

---

## References

- **Pattern**: Service Layer Integration
- **Trade-off**: Simplicity vs Decoupling
- **Related Package**: `github.com/vbonnet/engram/core/internal/tracking`

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)
