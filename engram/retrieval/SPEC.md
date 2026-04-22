# Retrieval Package - Specification

**Version**: 1.0.0
**Last Updated**: 2026-02-11
**Status**: Implemented
**Package**: github.com/vbonnet/engram/core/pkg/retrieval

---

## Vision

The retrieval package provides a high-level service layer for searching and retrieving engrams,
wrapping the ecphory 3-tier retrieval system with a user-friendly interface for CLI commands,
API servers, and programmatic access. It solves the problem of making engram search accessible
across different client types while maintaining consistent behavior and tracking usage patterns.

Terminal applications, REST APIs, and external tools need to search engrams with varying
requirements (different search paths, tag filters, API usage preferences). This package abstracts
the complexity of ecphory configuration, path resolution, and access tracking, allowing consumers
to focus on their specific use case while providing consistent search semantics.

---

## Goals

### 1. Simplified Search Interface

Provide a single, high-level Search API that handles all common engram retrieval scenarios
without exposing ecphory implementation details.

**Success Metric**: Clients can perform engram search with a single method call, passing
only the parameters relevant to their use case (query, tags, type, limit), without needing
to understand index building, ranker initialization, or token budgets.

### 2. Flexible Path Resolution

Support multiple engram path configurations: absolute paths, relative paths from cwd,
and default ~/.engram/core/engrams location, resolving automatically based on environment.

**Success Metric**: Users can specify engram paths as absolute, relative, or empty string
(default), and the service resolves to the correct directory without manual configuration.

### 3. Graceful API Degradation

Support both API-powered semantic ranking (when ANTHROPIC_API_KEY is available) and
index-only filtering (when API key is missing), falling back automatically without errors.

**Success Metric**: Search operations succeed in both API and non-API environments, with
automatic fallback when UseAPI is requested but API key is unavailable.

**Implementation**: When `SearchOptions.UseAPI` is true but `ANTHROPIC_API_KEY` environment
variable is not set, the service silently falls back to local-only index filtering without
returning an error. This ensures search operations always succeed regardless of API availability.

### 4. Access Tracking Integration

Track engram access patterns for metadata updates (last_accessed, access_count) to support
usage analytics and relevance scoring.

**Success Metric**: Every successful retrieval updates engram metadata via the tracking
subsystem, enabling downstream analytics without manual instrumentation.

---

## Architecture

### High-Level Design

The package acts as a **facade** over the ecphory retrieval system, adding path resolution,
error handling, access tracking, and result transformation. It follows a simple request-response
pattern with no persistent state beyond the parser and tracker instances.

```
┌─────────────────────┐
│   CLI / API / SDK   │ (Consumers)
└──────────┬──────────┘
           │ Uses
           v
      ┌─────────┐
      │ Service │──────┐ Owns
      └────┬────┘      │
           │ Delegates │
           │           v
      ┌────┴─────┐  ┌──────────┐
      │ Ecphory  │  │ Tracking │
      │ (3-tier) │  │ (Metrics)│
      └──────────┘  └──────────┘
           │
           v
      ┌──────────┐
      │  Engram  │ (File System)
      │  Files   │
      └──────────┘
```

### Components

**Component 1: Service**
- **Purpose**: Facade for engram retrieval operations
- **Responsibilities**:
  - Path resolution (resolveEngramPath)
  - Index building and candidate filtering
  - API ranking orchestration (with fallback)
  - Engram parsing and result transformation
  - Access tracking for retrieved engrams
  - Resource cleanup (Close method)
- **Interfaces**: Public API (`NewService`, `Search`, `Close`)

**Component 2: SearchOptions**
- **Purpose**: Configuration for search operations
- **Responsibilities**:
  - Define search parameters (query, tags, type, limit)
  - Enable API ranking flag (UseAPI)
  - Provide session context (SessionID, Transcript)
  - Specify engram directory path (EngramPath)
- **Interfaces**: Public struct with exported fields

**Component 3: SearchResult**
- **Purpose**: Container for search result data
- **Responsibilities**:
  - Store engram file path
  - Hold parsed engram object
  - Preserve ranking metadata (Score, Reasoning from API)
- **Interfaces**: Public struct with exported fields

### Data Flow

1. **Initialization**: Consumer calls `retrieval.NewService()`
   - Service creates engram.Parser for file parsing
   - Service creates tracking.Tracker for access metrics
   - No ecphory initialization (happens per-search for path flexibility)

2. **Search Request**: Consumer calls `service.Search(ctx, opts)`
   - Resolve engram path (absolute, relative, or default)
   - Build ecphory.Index from resolved path
   - Apply tier-1 filters (tags/type) to get candidates
   - If UseAPI: Attempt API ranking with fallback to index-only
   - Load and parse engram files for result paths
   - Record access tracking for each retrieved engram
   - Return SearchResult array with metadata

3. **Cleanup**: Consumer calls `service.Close()`
   - Flush tracking updates to disk
   - Release parser resources

### Key Design Decisions

- **Decision: Service owns tracker, not ecphory** (Improves separation of concerns)
  - Retrieval layer handles all telemetry/tracking concerns
  - Ecphory remains focused on search algorithm
  - Simplifies ecphory API (no EventBus injection required)

- **Decision: Per-search index building** (Supports dynamic paths)
  - Each Search() call builds fresh index from EngramPath
  - Enables different paths per search (multi-tenant, testing)
  - Trade-off: Higher cost for repeated searches (acceptable for CLI/API)

- **Decision: Automatic API fallback** (No user intervention required)
  - Missing API key → silently falls back to index-only
  - Ranker errors → fall back to index-only
  - Users always get results (degraded experience better than failure)

---

## Success Metrics

### Primary Metrics

- **API Simplicity**: 2 public methods (`NewService`, `Search`) + 1 cleanup (`Close`)
- **Zero Configuration**: Sensible defaults for all optional fields
- **Automatic Fallback**: API ranking errors never cause search failures

### Secondary Metrics

- **Path Resolution Coverage**: Handles absolute, relative, ~/.engram default, and nonexistent
- **Test Coverage**: ≥80% coverage on core logic (path resolution, filtering, API fallback)
- **Documentation**: Clear examples for CLI, API, and SDK usage

---

## What This SPEC Doesn't Cover

- **Caching**: No index caching across Search() calls (each search rebuilds index)
- **Concurrent Searches**: Service is not thread-safe (callers manage concurrency)
- **Custom Ranking**: No mechanism to override ranker behavior or scoring
- **Result Pagination**: No cursor-based pagination (only limit-based truncation)
- **Search History**: No storage of past searches or query logs
- **Incremental Index Updates**: Index is rebuilt from scratch each search

Future considerations:
- Index caching with TTL (performance optimization for repeated searches)
- Thread-safe Service variant (mutex-protected search)
- Custom ranker injection (dependency injection pattern)
- Cursor-based pagination for large result sets

---

## Assumptions & Constraints

### Assumptions

- Engram files use .ai.md extension and valid YAML frontmatter
- ANTHROPIC_API_KEY is set in environment for API ranking (optional - see fallback behavior below)
- Engram directories are read-accessible by service process
- Searches are infrequent enough that per-search index building is acceptable
- Callers handle Service lifecycle (Create → Search → Close)

**Note on API Fallback**: When `SearchOptions.UseAPI` is true but `ANTHROPIC_API_KEY` is not set,
the service automatically falls back to local-only index filtering. No error is returned; the search
succeeds using frontmatter-based filtering without semantic ranking. This ensures the service remains
functional in environments without API access.

### Constraints

- **Dependency Constraints**:
  - Requires `github.com/vbonnet/engram/core/pkg/ecphory` for retrieval
  - Requires `github.com/vbonnet/engram/core/pkg/engram` for parsing
  - Requires `github.com/vbonnet/engram/core/internal/tracking` for telemetry
- **Performance Constraints**:
  - Index rebuilt per search (O(n) file scan for n engrams)
  - API ranking adds latency (network round-trip to Anthropic)
  - No result streaming (entire result set loaded into memory)
- **Design Constraints**:
  - Single-threaded usage (no mutex protection)
  - No persistent state (stateless service)
  - Synchronous API (no async search)

---

## Dependencies

### External Libraries

- None (all dependencies are internal engram packages)

### Internal Dependencies

- `github.com/vbonnet/engram/core/pkg/ecphory` - 3-tier retrieval system
- `github.com/vbonnet/engram/core/pkg/engram` - Engram file parsing
- `github.com/vbonnet/engram/core/internal/tracking` - Access tracking and telemetry

---

## API Reference

### Types

```go
type SearchOptions struct {
    EngramPath  string   // Path to engrams directory
    Query       string   // Search query (semantic ranking via API)
    SessionID   string   // Session identifier for telemetry tracking
    Transcript  string   // Conversation transcript or context
    Tags        []string // Filter by tags (OR logic)
    Type        string   // Filter by type (pattern, strategy, workflow)
    Limit       int      // Maximum results to return
    UseAPI      bool     // Whether to use API ranking (requires ANTHROPIC_API_KEY)
}

type SearchResult struct {
    Path    string         // Full path to engram file
    Engram  *engram.Engram // Parsed engram
    Score   float64        // Relevance score (if API ranking used)
    Ranking string         // Reasoning (if API ranking used)
}

type Service struct {
    parser  *engram.Parser
    tracker *tracking.Tracker
}
```

### Functions

```go
// NewService creates a new retrieval service
func NewService() *Service
```

### Methods

```go
// Search performs engram retrieval with optional AI ranking
func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]*SearchResult, error)

// Close flushes pending tracking updates and cleans up resources
func (s *Service) Close() error
```

---

## Testing Strategy

### Unit Tests

**Coverage Areas**:
- Path resolution (absolute, relative, default, nonexistent)
- Candidate filtering (tags, type, no filters)
- Result limiting (0, negative, greater than candidates)
- API fallback (missing key, UseAPI=false)
- Parse error handling (skip unparseable engrams)

**Test Philosophy**:
- Focus on public API contracts (Search behavior, result structure)
- Use testutil.SetupTestEngrams for fixture creation
- Test both success and error paths

### Integration Tests

**Coverage Areas**:
- Full search pipeline (index build → filter → rank → parse → track)
- Custom search paths (non-default directories)
- Token budget limits (via result limit)
- Tag filtering with OR logic

**Test Scenarios**:
- Search with API key set (requires ANTHROPIC_API_KEY)
- Search without API key (fallback mode)
- Search with custom engram path
- Search with tag/type filters
- Search with result limit

---

## Version History

- **1.0.0** (2026-02-11): Initial implementation and documentation backfill

---

**Note**: This package is in active use by engram CLI and will be used by future API server.
Changes must maintain backward compatibility. See ARCHITECTURE.md for detailed design and
ADRs for decision rationale.
