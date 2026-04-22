# Retrieval Package - Architecture

**Version**: 1.0.0
**Last Updated**: 2026-02-11
**Status**: Implemented
**Package**: github.com/vbonnet/engram/core/pkg/retrieval

---

## Overview

The retrieval package provides a high-level service layer for engram search operations,
wrapping the ecphory 3-tier retrieval system with path resolution, API fallback handling,
access tracking, and result transformation.

**Key Principles**:
- Facade pattern over ecphory complexity
- Stateless per-search semantics (fresh index each search)
- Graceful degradation (API → index-only fallback)
- Integrated access tracking (every retrieval updates metrics)

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                     Consumer Layer                               │
│                                                                   │
│  CLI Commands:  engram retrieve --query "..." --tags go          │
│  API Server:    GET /api/search?q=...&tags=go                    │
│  SDK:           retrieval.NewService().Search(...)               │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             v
┌──────────────────────────────────────────────────────────────────┐
│                     Service (Facade)                             │
│                                                                   │
│  NewService() → Service{parser, tracker}                         │
│  Search(opts) → resolveEngramPath → buildIndex → filter →       │
│                 rank (optional) → parseEngrams → trackAccess     │
│  Close() → tracker.Flush()                                       │
└─────┬────────────────────────┬───────────────────────────────────┘
      │                        │
      │ Uses                   │ Uses
      v                        v
┌─────────────────┐      ┌──────────────────┐
│ Ecphory System  │      │ Tracking System  │
│                 │      │                  │
│ - Index         │      │ - Tracker        │
│ - Ranker        │      │ - MetadataUpdater│
│ - 3-Tier Query  │      │ - RecordAccess   │
└─────────────────┘      └──────────────────┘
      │                        │
      v                        v
┌─────────────────────────────────────┐
│         File System Layer           │
│                                      │
│  - *.ai.md engram files             │
│  - Frontmatter (YAML)               │
│  - Content (Markdown)               │
└─────────────────────────────────────┘
```

---

## Component Details

### 1. Service (Public API)

**File**: `retrieval.go`

**Purpose**: High-level facade for engram retrieval operations

**Responsibilities**:
- Encapsulate ecphory complexity (index building, ranker initialization, tier orchestration)
- Resolve engram directory paths (absolute, relative, default ~/.engram/core/engrams)
- Manage API ranking with graceful fallback (missing key → index-only)
- Parse retrieved engram files and transform to SearchResult objects
- Track access patterns for each retrieved engram
- Provide lifecycle management (Close for resource cleanup)

**Key Fields**:
```go
type Service struct {
    parser  *engram.Parser     // Parses .ai.md files
    tracker *tracking.Tracker  // Records access events
}
```

**Key Methods**:
```go
func NewService() *Service
    → Creates parser (engram.NewParser)
    → Creates tracker (tracking.NewTracker with MetadataUpdater)
    → Returns Service instance (no ecphory initialization yet)

func (s *Service) Search(ctx Context, opts SearchOptions) ([]*SearchResult, error)
    → Step 1: resolveEngramPath(opts.EngramPath)
    → Step 2: Build ecphory.Index from resolved path
    → Step 3: filterCandidates (by tags or type)
    → Step 4: If opts.UseAPI: rank candidates (with fallback)
    → Step 5: Parse engrams and build SearchResult objects
    → Step 6: trackAccess for each result
    → Returns result array or error

func (s *Service) Close() error
    → Flushes tracker updates (best-effort)
    → Returns nil (errors logged, not returned)
```

**Design Decisions**:
- **Why per-search index building?** → Supports dynamic EngramPath per search
- **Why tracker in Service not ecphory?** → Separation of concerns (retrieval vs metrics)
- **Why best-effort Close?** → Tracking is non-critical, shouldn't block cleanup

---

### 2. Path Resolution Logic

**Method**: `resolveEngramPath(path string) (string, error)`

**Purpose**: Resolve user-provided path to actual engram directory

**Resolution Strategy**:

1. **Absolute Path**: If `path` is absolute → verify exists → return as-is
2. **Default Path**: If `path` is empty → try `~/.engram/core/engrams` → return if exists
3. **Relative Path**: If `path` is relative → resolve from cwd → verify exists → return
4. **Error**: If none of above succeed → return error

**Flow Diagram**:
```
Input: opts.EngramPath

├─ Is absolute? ───YES──> Stat path ──EXISTS──> Return path
│                                    └─NOT EXISTS─> Error
│
├─ Is empty? ─────YES──> Try ~/.engram/core/engrams ──EXISTS──> Return default
│                                                     └─NOT EXISTS─> Try relative
│
└─ Is relative? ──YES──> Join(cwd, path) ──EXISTS──> Return full path
                                          └─NOT EXISTS─> Error
```

**Examples**:
- `/absolute/path/engrams` → Returns `/absolute/path/engrams` (if exists)
- `""` → Returns `~/.engram/core/engrams` (if exists) or tries `./engrams`
- `my-engrams` → Returns `<cwd>/my-engrams` (if exists)
- `/nonexistent` → Error: "engrams directory not found: /nonexistent"

**Design Decisions**:
- **Why try default before relative?** → Default is most common case for CLI usage
- **Why not create directory?** → Avoid accidental writes, fail fast on misconfiguration
- **Why return error on nonexistent?** → Better than silently returning empty results

---

### 3. Candidate Filtering Logic

**Method**: `filterCandidates(index *Index, opts SearchOptions) []string`

**Purpose**: Apply tier-1 filtering to narrow search space

**Filtering Strategy**:

1. **Tags Filter** (if `opts.Tags` provided): Return `index.FilterByTags(opts.Tags)`
   - OR logic: Any tag match includes the engram
   - Example: `["go", "python"]` matches engrams with go OR python tags

2. **Type Filter** (if `opts.Type` provided): Return `index.FilterByType(opts.Type)`
   - Exact match: Engram type must equal provided type
   - Example: `"pattern"` matches only type=pattern engrams

3. **No Filter** (if neither provided): Return `index.All()`
   - Returns all engrams in index (no filtering)

**Priority**: Tags filter takes precedence over type filter (if both provided, only tags used)

**Design Decisions**:
- **Why tags take precedence?** → Tags are more specific filter (type is broad category)
- **Why OR logic for tags?** → Matches common use case ("show me go OR python")
- **Why not AND logic option?** → Complexity not justified by current use cases

---

### 4. API Ranking with Fallback

**Code Section**: Lines 111-144 in `retrieval.go`

**Purpose**: Attempt semantic ranking via API, fall back to index-only on failure

**Fallback Logic**:
```
If opts.UseAPI:
    If ANTHROPIC_API_KEY not set:
        → Fallback to index-only (limitResults)
    Else:
        Create ecphory.Ranker
        If ranker creation fails:
            → Return error (API requested but unavailable)
        Call ranker.Rank(ctx, query, candidates)
        If ranking fails:
            → Return error (API call failed)
        Extract top N results based on opts.Limit
        Build rankings map for result metadata
Else:
    → Index-only (limitResults)
```

**Fallback Scenarios**:
- **No API Key**: Silent fallback to index-only (common in CI/CD)
- **Ranker Error**: Error returned (API was available but failed)
- **Ranking Error**: Error returned (API call failed mid-operation)

**Design Decisions**:
- **Why error on ranker creation failure?** → API was requested, user expects semantic search
- **Why silent fallback on missing key?** → Missing key is expected config state
- **Why not cache ranker?** → Ranker is stateless, per-search initialization is acceptable

---

### 5. Result Transformation

**Code Section**: Lines 146-172 in `retrieval.go`

**Purpose**: Convert ecphory results to SearchResult objects with parsed engrams

**Transformation Flow**:
```
For each result path:
    Parse engram file → s.parser.Parse(path)
    If parse error:
        → Skip engram (log warning, continue)
    Create SearchResult:
        - Path: full file path
        - Engram: parsed engram object
        - Score: from rankings map (if API used)
        - Ranking: reasoning from rankings map (if API used)
    Append to results array
    Track access: s.tracker.RecordAccess(path, now)
```

**Error Handling**: Parse errors are non-fatal (skip bad engrams, continue)

**Design Decisions**:
- **Why skip parse errors?** → Corrupt files shouldn't block search (degraded results OK)
- **Why track after parse?** → Only count successful retrievals (parsed engrams)
- **Why inline tracking?** → Simplifies code (no separate tracking loop)

---

### 6. Result Limiting

**Method**: `limitResults(candidates []string, limit int) []string`

**Purpose**: Truncate candidate list to specified limit

**Logic**:
```
If limit <= 0:
    → Return all candidates (no limit)
If limit >= len(candidates):
    → Return all candidates (limit doesn't apply)
Else:
    → Return candidates[:limit]
```

**Design Decisions**:
- **Why 0 means unlimited?** → Common convention (0 = no limit)
- **Why negative means unlimited?** → Defensive (avoid confusing negative behavior)
- **Why slice truncation?** → Simple, efficient (O(1) operation)

---

### 7. Access Tracking Integration

**Code Section**: Lines 169-170 in `retrieval.go`

**Purpose**: Record engram access for usage analytics

**Tracking Flow**:
```
For each successfully parsed result:
    s.tracker.RecordAccess(path, time.Now())
        → Queues update to tracking.Tracker
        → Tracker batches updates for flush

On Service.Close():
    s.tracker.Flush()
        → Writes pending updates to engram frontmatter
        → Updates last_accessed and access_count fields
```

**Tracking Metadata**:
- **last_accessed**: ISO 8601 timestamp of most recent access
- **access_count**: Incremented on each retrieval

**Design Decisions**:
- **Why track in retrieval not ecphory?** → Retrieval knows about file paths (ecphory only ranks)
- **Why best-effort flush?** → Tracking is non-critical (logged errors, no failures)
- **Why track after parse?** → Only count successful retrievals (not failed parse attempts)

---

## Data Flow Examples

### Example 1: API-Powered Search (Happy Path)

```
User: engram retrieve --query "error handling" --tags go --use-api

Service.Search(ctx, SearchOptions{
    EngramPath: "",
    Query:      "error handling",
    Tags:       ["go"],
    UseAPI:     true,
    Limit:      10,
})
    │
    ├─> resolveEngramPath("") → "~/.engram/core/engrams"
    │
    ├─> ecphory.Index.Build("~/.engram/core/engrams") → Index with 100 engrams
    │
    ├─> filterCandidates(index, opts) → 15 engrams with "go" tag
    │
    ├─> Check ANTHROPIC_API_KEY → Found
    │
    ├─> ecphory.NewRanker() → Ranker instance
    │
    ├─> ranker.Rank(ctx, "error handling", [15 paths]) → Ranked results
    │       │
    │       └─> API Call: Anthropic Claude 3.5 Sonnet
    │           Returns: [{path: "go-errors.ai.md", score: 0.95, reasoning: "..."}]
    │
    ├─> Take top 10 results (limit)
    │
    ├─> Parse engrams:
    │   - go-errors.ai.md → Engram object
    │   - go-panic.ai.md → Engram object
    │   - ...
    │
    ├─> Track access for each result
    │   - RecordAccess("go-errors.ai.md", 2026-02-11T10:30:00Z)
    │   - RecordAccess("go-panic.ai.md", 2026-02-11T10:30:00Z)
    │
    └─> Return [SearchResult{Path, Engram, Score, Ranking}, ...]

User: service.Close()
    └─> tracker.Flush() → Writes metadata updates to frontmatter
```

### Example 2: Index-Only Search (Fallback)

```
User: engram retrieve --query "testing" --tags python
(No --use-api flag, or ANTHROPIC_API_KEY not set)

Service.Search(ctx, SearchOptions{
    EngramPath: "/tmp/test-engrams",
    Tags:       ["python"],
    UseAPI:     false,
    Limit:      5,
})
    │
    ├─> resolveEngramPath("/tmp/test-engrams") → "/tmp/test-engrams"
    │
    ├─> ecphory.Index.Build("/tmp/test-engrams") → Index with 10 engrams
    │
    ├─> filterCandidates(index, opts) → 4 engrams with "python" tag
    │
    ├─> opts.UseAPI == false → Skip API ranking
    │
    ├─> limitResults(candidates, 5) → All 4 candidates (under limit)
    │
    ├─> Parse engrams:
    │   - pytest.ai.md → Engram object
    │   - python-mock.ai.md → Engram object
    │   - ...
    │
    ├─> Track access for each result
    │
    └─> Return [SearchResult{Path, Engram, Score=0, Ranking=""}, ...]
        (No score/ranking in index-only mode)
```

### Example 3: Parse Error Handling

```
Service.Search(ctx, SearchOptions{EngramPath: "/engrams", Limit: 10})
    │
    ├─> Index built → 3 engram files
    │
    ├─> Parse loop:
    │   - valid1.ai.md → SUCCESS → Add to results
    │   - invalid.ai.md → PARSE ERROR (malformed YAML) → Skip, continue
    │   - valid2.ai.md → SUCCESS → Add to results
    │
    └─> Return [result1, result2]
        (invalid.ai.md excluded from results)
```

---

## Threading Model

**Single-Threaded by Design**: The retrieval service is **not thread-safe**.

**Rationale**:
- Service has no shared mutable state (parser and tracker are independent)
- Each Search() call creates fresh ecphory.Index (no shared state)
- Most use cases are single-threaded (CLI, sequential API requests)

**Recommendation for Concurrent Usage**:
- Create separate Service instances per goroutine
- OR synchronize Search() calls with external mutex
- OR use service pool pattern (channel of Service instances)

---

## Error Handling

**Philosophy**: Fail fast on configuration errors, gracefully degrade on data errors.

**Error Categories**:

1. **Configuration Errors** (return error):
   - Engram path not found (`resolveEngramPath` failure)
   - Index build failure (`index.Build` failure)
   - Ranker initialization failure (API requested but ranker creation failed)
   - Ranking failure (API call failed)

2. **Data Errors** (skip and continue):
   - Parse errors (malformed YAML, invalid frontmatter)
   - Individual engram read failures

3. **Non-Critical Errors** (log and ignore):
   - Tracking flush failures (`Close` method)

**Examples**:
- `Search("/nonexistent", ...)` → Error: "engrams directory not found"
- `Search` with malformed engram → Skip engram, return partial results
- `Close()` tracking failure → Log warning, return nil

---

## Testing Strategy

### Unit Tests (retrieval_test.go)

**Coverage Areas**:
- Path resolution (absolute, relative, default, nonexistent)
- Candidate filtering (tags OR logic, type filter, no filters)
- Result limiting (zero, negative, greater than candidates)
- API fallback (missing key, UseAPI=false)
- Parse error handling (skip unparseable engrams)

**Test Philosophy**:
- Use testutil.SetupTestEngrams for fixture creation
- Test public API contracts (Search behavior, result structure)
- Mock-free (real index, real parser, real files)

### Integration Tests (retrieval_integration_test.go)

**Coverage Areas**:
- Full search pipeline (custom paths, tag filters, token limits)
- Config integration (search paths, limits)
- Real ecphory.Index and ranking (requires ANTHROPIC_API_KEY)

**Test Scenarios**:
- Custom search paths (non-default directories)
- Token budget limits (via result limit)
- Tag filtering with OR logic
- API-powered ranking (skipped if no API key)

---

## Dependencies

### External Dependencies

None (all dependencies are internal engram packages)

### Internal Dependencies

**github.com/vbonnet/engram/core/pkg/ecphory**
- Used by: Service.Search
- Purpose: 3-tier retrieval (index, rank, budget)

**github.com/vbonnet/engram/core/pkg/engram**
- Used by: Service (parser field)
- Purpose: Parse .ai.md files into Engram objects

**github.com/vbonnet/engram/core/internal/tracking**
- Used by: Service (tracker field)
- Purpose: Record engram access events

---

## Performance Considerations

### Memory Usage

**Service**: ~1 KB (parser + tracker references)
**Index**: ~100 bytes per engram (frontmatter cache)
**Results**: ~10 KB per SearchResult (parsed engram + metadata)

**Example**: Searching 1000 engrams with 10 results:
- Index: ~100 KB
- Results: ~100 KB
- Total: ~200 KB (negligible)

### CPU Usage

**Index Building**: O(n) file scan for n engrams (~1ms per engram)
**Filtering**: O(n) frontmatter scan (~0.1ms per engram)
**Parsing**: O(m) file reads for m results (~5ms per engram)
**API Ranking**: O(1) network call (~500ms)

**Example**: Search 1000 engrams, return 10 results:
- Index build: ~1s
- Filter: ~100ms
- API rank: ~500ms
- Parse: ~50ms
- **Total**: ~1.6s (dominated by API call)

### Optimization Opportunities

**Index Caching**: Cache index per EngramPath with TTL (avoid rebuild on repeated searches)
**Lazy Parsing**: Parse only top N results (skip unused candidates)
**Parallel Parsing**: Parse results in parallel (goroutines + channel)

---

## Future Enhancements

### Potential Improvements (Not in Scope for v1.0.0)

**Index Caching**:
- Currently: Index rebuilt per search
- Future: Cache index with TTL, invalidate on file changes

**Thread Safety**:
- Currently: Not thread-safe
- Future: Mutex-protected Search method (opt-in via config)

**Cursor-Based Pagination**:
- Currently: Limit-based truncation
- Future: Cursor tokens for stateful pagination

**Custom Ranker Injection**:
- Currently: Hard-coded ecphory.Ranker
- Future: Ranker interface with DI (custom scoring algorithms)

**Search History**:
- Currently: No query logging
- Future: Optional search history for analytics

---

## Version History

- **1.0.0** (2026-02-11): Initial implementation and architecture documentation backfill

---

**Maintained by**: Engram Core Team
**Questions**: See README.md for usage examples, SPEC.md for requirements
