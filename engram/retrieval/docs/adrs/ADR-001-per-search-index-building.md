# ADR-001: Per-Search Index Building

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Initial design of retrieval service

---

## Context

The retrieval service wraps the ecphory system to provide engram search capabilities. A key
decision is when to build the ecphory.Index: at Service initialization (persistent index) or
per Search call (ephemeral index).

**Trade-offs**:
- **Persistent Index**: Faster searches, but requires index invalidation on file changes, and
  ties Service to single EngramPath
- **Per-Search Index**: Slower searches, but supports dynamic paths and avoids cache invalidation

**Use Cases**:
1. **CLI**: Single search per invocation, different paths per user
2. **API Server**: Multiple searches, potentially different paths per request (multi-tenant)
3. **SDK**: Programmatic use, varying paths based on application logic

---

## Decision

We will build the ecphory.Index **per Search call**, not at Service initialization.

**Implementation**:
```go
func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
    // Resolve path
    engramPath, err := s.resolveEngramPath(opts.EngramPath)
    if err != nil {
        return nil, err
    }

    // Build fresh index for this search
    index := ecphory.NewIndex()
    if err := index.Build(engramPath); err != nil {
        return nil, fmt.Errorf("failed to build index: %w", err)
    }

    // Use index for this search...
}
```

**Characteristics**:
- Index lifetime: Single Search call
- Index scope: Specific to opts.EngramPath
- No caching between searches
- No invalidation logic required

---

## Consequences

### Positive

**Dynamic Path Support**:
- Each Search can specify different EngramPath
- Supports multi-tenant scenarios (search different engram collections)
- Enables testing with temporary directories (no shared state pollution)

**No Cache Invalidation**:
- No need to detect file changes (index is always fresh)
- No TTL logic or invalidation heuristics
- No stale index bugs (every search sees current state)

**Simpler Service Lifecycle**:
- NewService() is lightweight (no index building)
- No Close() cleanup for index (index is garbage collected)
- No thread-safety concerns for shared index

**Clear Semantics**:
- Each Search is independent (no hidden dependencies)
- Easy to reason about (no global state)
- Testable in isolation (no setup/teardown for shared index)

### Negative

**Performance Overhead**:
- Every Search pays O(n) cost to scan engram files
- Repeated searches re-scan same files
- Example: 1000 engrams × 1ms/engram = 1s overhead per search

**Memory Churn**:
- Index allocated and freed each search
- Garbage collection pressure for frequent searches
- No memory reuse across searches

**No Warm-Up Optimization**:
- First search is as slow as subsequent searches
- Can't pre-build index for known paths
- API server pays full cost on every request

### Mitigation Strategies

**For CLI**: Acceptable (single search per invocation, overhead amortized)

**For API Server**: Add caching layer if needed:
```go
type CachedService struct {
    service   *Service
    indexCache map[string]*cacheEntry // path → {index, timestamp}
    cacheTTL   time.Duration
}
```

**For High-Frequency Use**: Recommend creating index manually:
```go
// Advanced users can bypass retrieval.Service
index := ecphory.NewIndex()
index.Build(engramPath)
for i := 0; i < 1000; i++ {
    results := index.FilterByTags(tags)
    // Use results...
}
```

---

## Alternatives Considered

### Alternative 1: Persistent Index at Service Init

**Approach**:
```go
func NewService(engramPath string) (*Service, error) {
    index := ecphory.NewIndex()
    if err := index.Build(engramPath); err != nil {
        return nil, err
    }
    return &Service{index: index}, nil
}
```

**Rejected Because**:
- Ties Service to single EngramPath (no dynamic paths)
- Requires cache invalidation (when to rebuild?)
- Complex lifecycle (index in Service state)
- Thread-safety concerns (shared index)

### Alternative 2: Index Cache with TTL

**Approach**:
```go
func (s *Service) Search(opts SearchOptions) ([]*SearchResult, error) {
    index := s.getOrBuildIndex(opts.EngramPath) // Cache with 5min TTL
    // Use index...
}
```

**Rejected Because**:
- Complexity: TTL logic, cache eviction, thread-safety
- Uncertain benefit: CLI doesn't benefit (single search)
- Premature optimization: No evidence of performance bottleneck
- Can be added later if needed (doesn't break API)

### Alternative 3: Lazy Initialization

**Approach**:
```go
func NewService(engramPath string) *Service {
    return &Service{engramPath: engramPath} // No index yet
}

func (s *Service) Search(opts SearchOptions) ([]*SearchResult, error) {
    if s.index == nil {
        s.buildIndex() // Build on first search
    }
    // Use index...
}
```

**Rejected Because**:
- Still ties Service to single path (no dynamic paths)
- Thread-safety issues (lazy init in concurrent searches)
- Unclear semantics (when is index rebuilt?)

---

## Implementation Notes

**Index Building Code** (lines 95-99 in `retrieval.go`):
```go
// Build index
index := ecphory.NewIndex()
if err := index.Build(engramPath); err != nil {
    return nil, fmt.Errorf("failed to build index: %w", err)
}
```

**Performance Characteristics**:
- Build cost: O(n) where n = number of .ai.md files
- Typical build time: ~1ms per engram (1000 engrams = ~1s)
- Memory usage: ~100 bytes per engram (1000 engrams = ~100 KB)

**Future Optimization**:
If performance becomes an issue, add caching layer in API server (not in retrieval package):
```go
// api-server/search_handler.go
type searchHandler struct {
    service   *retrieval.Service
    indexCache *lru.Cache // LRU cache of indices
}
```

---

## Related Decisions

- **ADR-002**: Tracking Integration in Service Layer (affected by per-search semantics)
- **ADR-003**: API Ranking Fallback Strategy (affected by stateless service design)

---

## References

- **Pattern**: Ephemeral vs Persistent State
- **Trade-off**: Performance vs Flexibility
- **Related Code**: `ecphory.Index.Build` implementation

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)
