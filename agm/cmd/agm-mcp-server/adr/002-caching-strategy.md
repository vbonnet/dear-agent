# ADR 002: Caching Strategy

## Status

Accepted

## Context

The AGM MCP server needs to query session metadata from the filesystem (`~/.config/agm/sessions/`). Each session has a `manifest.json` file containing metadata. With 100-1000 sessions, reading all manifest files on every query would result in unacceptable latency.

### Performance Requirements

| Session Count | p99 Target |
|--------------|-----------|
| 100 sessions | <50ms |
| 500 sessions | <80ms |
| 1000 sessions | <100ms |

### Workload Characteristics

- **Read-Heavy**: 99% reads, <1% writes (sessions rarely created during queries)
- **Query Patterns**: List all → Filter → Search → Get details (common flow)
- **Temporal Locality**: Users often query same session multiple times in succession
- **Freshness Tolerance**: Session metadata changes infrequently (minutes to hours)

### Options Considered

#### Option 1: No Caching (Read from Disk Every Time)

**Implementation**:
```go
func listSessions(dir string) ([]*manifest.Manifest, error) {
    return manifest.List(dir) // Reads all manifest.json files
}
```

**Pros**:
- Simple implementation
- Always fresh data
- No memory overhead

**Cons**:
- Disk I/O on every query (100-1000 file reads)
- Latency scales linearly with session count
- Cannot meet p99 targets for 500+ sessions

**Benchmark Estimate**:
- 100 sessions: ~80ms (fails <50ms target)
- 500 sessions: ~400ms (fails <80ms target)
- 1000 sessions: ~800ms (fails <100ms target)

#### Option 2: In-Memory Cache with Fixed TTL

**Implementation**:
```go
var (
    cache      []*manifest.Manifest
    cacheTime  time.Time
    cacheMutex sync.RWMutex
)

func listSessionsCached(dir string) ([]*manifest.Manifest, error) {
    cacheMutex.RLock()
    if time.Since(cacheTime) < TTL {
        defer cacheMutex.RUnlock()
        return cache, nil
    }
    cacheMutex.RUnlock()

    cacheMutex.Lock()
    defer cacheMutex.Unlock()

    // Refresh cache
    sessions, err := manifest.List(dir)
    if err != nil {
        return nil, err
    }
    cache = sessions
    cacheTime = time.Now()
    return sessions, nil
}
```

**Pros**:
- Sub-millisecond latency on cache hit
- Meets all performance targets
- Simple implementation (30 lines)
- Thread-safe with RWMutex

**Cons**:
- Stale data for up to TTL duration
- Memory overhead (~1MB per 1000 sessions)
- Cache stampede risk (mitigated with double-check locking)

**Benchmark Estimate**:
- Cache hit: <1ms (meets all targets)
- Cache miss: Same as Option 1, but infrequent (1/TTL)

#### Option 3: Filesystem Watcher with Event-Driven Invalidation

**Implementation**:
```go
func watchSessions(dir string) {
    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(dir)
    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                invalidateCache()
            }
        }
    }
}
```

**Pros**:
- Always fresh (invalidates on file changes)
- No TTL staleness
- Efficient (only refreshes when needed)

**Cons**:
- Complex implementation (goroutine, error handling)
- Dependency on `fsnotify` library
- Recursive watch needed for subdirectories
- Potential for missed events (kernel buffer overflow)
- Overkill for V1 (sessions rarely change during queries)

#### Option 4: LRU Cache with Per-Session Granularity

**Implementation**:
```go
type SessionCache struct {
    sessions map[string]*manifest.Manifest // sessionID → manifest
    lru      *list.List                     // LRU eviction order
}
```

**Pros**:
- Memory-efficient (only cache frequently accessed sessions)
- Granular invalidation (per session)

**Cons**:
- Complex implementation (LRU bookkeeping)
- Doesn't help list/search queries (need full session list)
- Over-engineered for current use case

## Decision

We will use **Option 2: In-Memory Cache with Fixed TTL (5 seconds)**.

## Rationale

### Why Fixed TTL Cache?

1. **Simplicity**: 30 lines of code, easy to understand and maintain
2. **Performance**: Meets all p99 targets with sub-millisecond cache hits
3. **Correctness**: Thread-safe with double-check locking pattern
4. **Pragmatism**: 5s staleness is acceptable for session metadata (changes infrequently)

### Why 5 Second TTL?

- **Freshness**: Short enough that users won't notice stale data
- **Performance**: Long enough to avoid frequent disk I/O
- **Balance**: Typical query session (list → search → get) completes in <5s

### Why Not Filesystem Watcher?

- **Complexity**: Adds significant code complexity for marginal benefit
- **V1 Scope**: Sessions rarely change during active queries (create/archive is infrequent)
- **Future**: Can add in V2 if staleness becomes an issue

### Why Not LRU Cache?

- **Query Pattern**: Most queries need full session list (list/search), not individual sessions
- **Memory**: 1MB for 1000 sessions is acceptable for V1
- **Optimization**: Premature optimization (no evidence LRU is needed)

## Implementation Details

### Cache Structure

```go
var (
    sessionListCache []*manifest.Manifest  // Cached session list
    cacheTimestamp   time.Time             // Last refresh time
    cacheMutex       sync.RWMutex          // Concurrency control
)
```

### Cache Algorithm

1. **Read Path** (Fast Path):
   - Acquire read lock
   - Check cache age (<5s?)
   - Return cached data if fresh
   - Release read lock

2. **Refresh Path** (Slow Path):
   - Release read lock
   - Acquire write lock
   - Double-check cache age (race condition)
   - Read from disk if still stale
   - Update cache + timestamp
   - Release write lock

### Double-Check Locking

Prevents cache stampede when multiple requests arrive during refresh:
```go
cacheMutex.RLock()
if time.Since(cacheTimestamp) < 5*time.Second {
    return cache, nil  // Fast path
}
cacheMutex.RUnlock()

cacheMutex.Lock()
defer cacheMutex.Unlock()

// Double-check: another goroutine may have refreshed
if time.Since(cacheTimestamp) < 5*time.Second {
    return cache, nil  // Another thread refreshed
}

// Refresh cache
cache = manifest.List(dir)
cacheTimestamp = time.Now()
```

### Thread Safety

- `sync.RWMutex` allows concurrent reads (list, search, get can run in parallel)
- Exclusive write lock for cache refresh (only one thread reads disk)
- No data races (cache and timestamp protected by same mutex)

## Consequences

### Positive

- **Performance**: Meets all p99 latency targets
- **Simplicity**: Minimal code, easy to debug
- **Scalability**: Handles 1000+ sessions efficiently
- **Concurrency**: Allows parallel query processing

### Negative

- **Staleness**: Data can be up to 5s stale
- **Memory**: Keeps all sessions in memory (~1MB per 1000)
- **Cold Start**: First query after 5s incurs disk I/O latency

### Neutral

- **Cache Invalidation**: V1 has no invalidation (sessions rarely change during queries)
- **Eviction**: No eviction policy (cache entire session list)

## Monitoring

### Metrics to Track (V2)

- Cache hit rate (should be >90%)
- Cache refresh latency (disk I/O time)
- Memory usage (session list size)
- Staleness incidents (queries returning stale data)

### Logging

- Cache miss: Log to stderr with timestamp
- Cache refresh: Log time taken for disk I/O

## Future Enhancements

### V2: Cache Invalidation API

Add explicit invalidation when sessions are created/archived:
```go
func invalidateCache() {
    cacheMutex.Lock()
    defer cacheMutex.Unlock()
    cacheTimestamp = time.Time{} // Force refresh on next query
}
```

Call from AGM CLI when modifying sessions.

### V3: Filesystem Watcher

If staleness becomes an issue, add `fsnotify` watcher:
- Watch `~/.config/agm/sessions/` directory
- Invalidate cache on manifest.json changes
- Requires recursive watch for subdirectories

### V4: Distributed Cache

For multi-machine AGM setups (remote sessions):
- Use Redis or similar for shared cache
- Pub/sub for invalidation across machines
- Requires network transport (HTTP/gRPC)

## Benchmarks

### Expected Performance

| Scenario | Latency | Notes |
|----------|---------|-------|
| Cache hit (100 sessions) | <1ms | Memory read |
| Cache hit (1000 sessions) | <1ms | Memory read |
| Cache miss (100 sessions) | ~80ms | Disk I/O |
| Cache miss (1000 sessions) | ~800ms | Disk I/O |

### Cache Hit Rate Projection

Assuming queries arrive randomly:
- TTL = 5s
- Query rate = 1 query/second
- Hit rate = (5s - 1/rate) / 5s = 80%

With bursty queries (list → search → get in <5s):
- Hit rate ≈ 95% (first query misses, rest hit)

## Alternatives for Future

If requirements change, we can switch caching strategies:

1. **Longer TTL**: Increase to 30s if freshness tolerance increases
2. **Shorter TTL**: Decrease to 1s if staleness becomes an issue
3. **No TTL**: Use filesystem watcher for always-fresh data
4. **LRU Cache**: If memory becomes constrained with >10,000 sessions

## References

- Double-Check Locking: https://en.wikipedia.org/wiki/Double-checked_locking
- Go sync.RWMutex: https://pkg.go.dev/sync#RWMutex
- Cache Stampede: https://en.wikipedia.org/wiki/Cache_stampede

## Decision Date

2025-01-15

## Reviewers

- vbonnet (author)
