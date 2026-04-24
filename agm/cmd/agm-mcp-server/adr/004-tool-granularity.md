# ADR 004: Tool Granularity

## Status

Accepted

## Context

The AGM MCP server needs to expose session querying capabilities to MCP clients. We need to decide how to structure these capabilities as MCP tools.

### Core Operations

1. **List sessions**: Get all sessions with optional filters
2. **Search sessions**: Find sessions by name
3. **Get session**: Retrieve specific session by ID

### Options Considered

#### Option 1: Single Monolithic Tool

**Structure**:
```json
{
  "name": "agm_query",
  "input": {
    "operation": "list|search|get",
    "filters": {...},
    "query": "...",
    "session_id": "..."
  }
}
```

**Pros**:
- Single tool to document/maintain
- Flexible (can add operations without new tools)
- Fewer tools to register

**Cons**:
- Complex input schema (union of all operation inputs)
- Poor discoverability (AI must read docs to know operations)
- Validation complexity (different fields required per operation)
- Violates Single Responsibility Principle

#### Option 2: Fine-Grained Tools (One Per Operation + Filter)

**Structure**:
```
agm_list_sessions
agm_list_active_sessions
agm_list_archived_sessions
agm_search_sessions
agm_search_active_sessions
agm_search_archived_sessions
agm_get_session
```

**Pros**:
- Very specific, clear purpose per tool
- Minimal input validation
- Self-documenting

**Cons**:
- Tool explosion (7+ tools for basic operations)
- Duplication (similar logic across tools)
- Poor extensibility (new filter = new tools)
- Overwhelming for users (too many choices)

#### Option 3: Three Focused Tools (Recommended)

**Structure**:
```
agm_list_sessions
  - Filters: status, agent_type, limit

agm_search_sessions
  - Query: search string
  - Filters: status, limit

agm_get_session_metadata
  - SessionID: required UUID
```

**Pros**:
- Clear separation of concerns (list vs search vs get)
- Flexible filtering (filters as input params, not separate tools)
- Discoverable (3 tools, obvious purposes)
- Extensible (add filters without new tools)
- Aligns with REST principles (list, search, get)

**Cons**:
- Slight duplication (status filter in both list and search)

## Decision

We will use **Option 3: Three Focused Tools**.

## Rationale

### Separation of Concerns

Each tool has a single, clear purpose:
- **list**: Get all sessions (with optional filters)
- **search**: Find sessions by name query
- **get**: Retrieve specific session by ID

This maps naturally to user intent:
- "Show me all my sessions" → list
- "Find my session about X" → search
- "What's the status of session Y?" → get

### Discoverability

Claude Code can discover tools via MCP protocol:
```
Available tools:
- agm_list_sessions: List AGM sessions with filters
- agm_search_sessions: Search AGM sessions by name
- agm_get_session_metadata: Get detailed metadata for a session
```

An AI can understand these without reading detailed docs.

### Input Validation

Each tool has a focused input schema:

**List**:
```go
type ListSessionsInput struct {
    Filters struct {
        Status    string // active|archived|all
        AgentType string // claude|all
        Limit     int    // max 1000
    }
}
```

**Search**:
```go
type SearchSessionsInput struct {
    Query   string // required
    Filters struct {
        Status string // active|archived|all
        Limit  int    // max 50
    }
}
```

**Get**:
```go
type GetSessionMetadataInput struct {
    SessionID string // required UUID
}
```

Clear required fields, no ambiguity.

### Extensibility

Adding filters doesn't require new tools:
```yaml
# V2: Add "project" filter
filters:
  status: active
  agent_type: claude
  project: my-project  # New filter, no new tool
```

### Standard Pattern

This matches common API patterns:
- **REST**: GET /sessions, GET /sessions/search, GET /sessions/:id
- **GraphQL**: sessions(filters), searchSessions(query), session(id)
- **gRPC**: ListSessions, SearchSessions, GetSession

Familiar to developers.

## Tool Specifications

### Tool 1: agm_list_sessions

**Purpose**: List all sessions with optional filtering

**Use Cases**:
- "Show me all active sessions"
- "List my 10 most recent sessions"
- "Get all archived sessions"

**Input Schema**:
```json
{
  "filters": {
    "status": "active|archived|all",
    "agent_type": "claude|all",
    "limit": 100
  }
}
```

**Output Schema**:
```json
{
  "sessions": [...],
  "total_count": 150,
  "filtered_count": 100
}
```

**Constraints**:
- `limit` max: 1000 (prevent memory exhaustion)
- Default `limit`: 100 (reasonable default)
- Default `status`: all (most permissive)

### Tool 2: agm_search_sessions

**Purpose**: Search sessions by name with relevance ranking

**Use Cases**:
- "Find my session about refactoring auth"
- "Search for sessions containing 'MCP'"
- "What sessions mention 'performance'?"

**Input Schema**:
```json
{
  "query": "search string",
  "filters": {
    "status": "active|archived|all",
    "limit": 10
  }
}
```

**Output Schema**:
```json
{
  "sessions": [
    {
      "...metadata": "...",
      "relevance_score": 0.95
    }
  ],
  "total_matches": 5
}
```

**Constraints**:
- `query` is required (no query = use list instead)
- `limit` max: 50 (search results should be focused)
- Default `limit`: 10 (top 10 results)
- Results sorted by relevance (desc)

### Tool 3: agm_get_session_metadata

**Purpose**: Get detailed metadata for specific session

**Use Cases**:
- "What's the status of session abc-123?"
- "When was session xyz created?"
- "Show me details for this session"

**Input Schema**:
```json
{
  "session_id": "uuid"
}
```

**Output Schema**:
```json
{
  "session": {
    "id": "uuid",
    "session_name": "...",
    "created_at": "...",
    "updated_at": "...",
    "status": "active",
    "agent_type": "claude",
    "tmux_session": "..."
  }
}
```

**Constraints**:
- `session_id` is required (must be valid UUID)
- Returns error if not found

## Why Not More Tools?

### Rejected: agm_list_active_sessions

Use `agm_list_sessions` with `filters.status = "active"` instead.

**Reason**: Filters should be parameters, not separate tools. Otherwise we'd need:
- agm_list_active_claude_sessions
- agm_list_archived_claude_sessions
- agm_list_active_gemini_sessions
- ... (combinatorial explosion)

### Rejected: agm_count_sessions

Use `agm_list_sessions` and read `total_count` field.

**Reason**: No need for separate tool when list returns count.

### Rejected: agm_get_recent_sessions

Use `agm_list_sessions` with `filters.limit = N`.

**Reason**: Manifest.List() already sorts by updated_at descending.

## Why Not Fewer Tools?

### Rejected: Merge list + search into agm_query_sessions

**Problem**: Ambiguous input schema:
```json
{
  "query": "...",    // Required for search, ignored for list
  "filters": {...}   // Different constraints for list vs search
}
```

**Result**: AI must know when to use `query` vs `filters`, unclear intent.

### Rejected: Merge get into list with filter

**Problem**: Different return shapes:
- list returns `{sessions: [...], total_count, filtered_count}`
- get returns `{session: {...}}`

**Result**: Inconsistent API, harder to use.

## Implementation Notes

### Tool Registration

```go
// main.go
func main() {
    server := mcp.NewServer(...)

    addListSessionsTool(server, cfg)
    addSearchSessionsTool(server, cfg)
    addGetSessionMetadataTool(server, cfg)

    server.Run(ctx, transport)
}
```

### Shared Code

All tools share:
- Cache layer: `listSessionsCached()`
- Transform layer: `manifestToMCPMetadata()`
- Filter helpers: `filterSessions()`, `searchSessionsByName()`

### Code Deduplication

Filter logic is shared across tools:
```go
func filterSessions(sessions []*manifest.Manifest, status string, agentType string) []*manifest.Manifest {
    // Shared by list and search
}
```

## Consequences

### Positive

- **Clear Intent**: Each tool maps to distinct user intent
- **Simple Validation**: Each tool has focused input schema
- **Extensible**: Add filters without new tools
- **Discoverable**: AI can understand 3 tools easily
- **Testable**: Each tool tested independently

### Negative

- **Filter Duplication**: `status` filter appears in list and search
- **Code Duplication**: Similar validation logic across tools

### Mitigation

Share filter logic via helper functions:
```go
// tools.go
func filterSessions(...) {...}  // Used by list and search
func validateLimit(limit, max int) error {...}  // Used by all tools
```

## Future Tools (V2+)

### V2: Session Modification Tools

- `agm_create_session`: Create new session
- `agm_archive_session`: Archive existing session
- `agm_update_session_metadata`: Update name, tags

### V3: Bulk Operations

- `agm_bulk_archive_sessions`: Archive multiple sessions
- `agm_export_sessions`: Export session metadata as JSON

### V4: Analytics Tools

- `agm_get_session_stats`: Usage statistics across sessions
- `agm_get_activity_timeline`: Session activity over time

## Alternatives Considered for Future

If tool count grows beyond 10, consider:
- **Namespacing**: `agm.sessions.list`, `agm.sessions.search`
- **Operation Parameter**: Single tool with `operation` field
- **Tool Groups**: Separate MCP servers for read vs write operations

## References

- REST API Design: https://restfulapi.net/
- MCP Tool Best Practices: https://modelcontextprotocol.io/docs/tools
- Single Responsibility Principle: https://en.wikipedia.org/wiki/Single-responsibility_principle

## Decision Date

2025-01-15

## Reviewers

- vbonnet (author)
