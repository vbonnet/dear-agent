# AGM MCP Server - Architecture

## System Overview

The AGM MCP Server is a lightweight MCP (Model Context Protocol) server that bridges AGM session metadata with external MCP clients like Claude Code. It runs as a standalone process, communicates via stdio, and provides read-only access to session metadata.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        Claude Code (MCP Client)                 │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  MCP Client SDK                                           │  │
│  │  - JSON-RPC 2.0 over stdio                               │  │
│  │  - Tool discovery & invocation                           │  │
│  └──────────────────┬───────────────────────────────────────┘  │
└─────────────────────┼───────────────────────────────────────────┘
                      │
                      │ stdin/stdout (JSON-RPC)
                      │ stderr (logs)
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                    AGM MCP Server Process                        │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ main.go - Entry Point                                     │  │
│  │ - Server initialization                                   │  │
│  │ - Tool registration                                       │  │
│  │ - Transport setup (stdio)                                │  │
│  │ - Logging to stderr                                      │  │
│  └──────────────────┬───────────────────────────────────────┘  │
│                     │                                           │
│  ┌──────────────────▼───────────────────────────────────────┐  │
│  │ config.go - Configuration Management                      │  │
│  │ - YAML parsing                                            │  │
│  │ - Smart defaults                                          │  │
│  │ - Environment detection                                   │  │
│  └──────────────────┬───────────────────────────────────────┘  │
│                     │                                           │
│  ┌──────────────────▼───────────────────────────────────────┐  │
│  │ tools.go - MCP Tool Handlers                              │  │
│  │ - agm_list_sessions                                       │  │
│  │ - agm_search_sessions                                     │  │
│  │ - agm_get_session_metadata                                │  │
│  └─────┬─────────────────────────────────┬──────────────────┘  │
│        │                                 │                      │
│  ┌─────▼─────────────────┐  ┌───────────▼──────────────────┐  │
│  │ cache.go              │  │ transform.go                  │  │
│  │ - Session list cache  │  │ - Manifest → MCP conversion   │  │
│  │ - 5s TTL              │  │ - Relevance scoring           │  │
│  │ - Thread-safe         │  │ - Lifecycle mapping           │  │
│  └─────┬─────────────────┘  └───────────┬──────────────────┘  │
│        │                                 │                      │
└────────┼─────────────────────────────────┼──────────────────────┘
         │                                 │
         │                                 │
         ▼                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│              AGM Internal Libraries                              │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ internal/manifest                                         │  │
│  │ - manifest.List(sessionsDir)                              │  │
│  │ - manifest.Manifest struct                                │  │
│  │ - Session metadata parsing                                │  │
│  └──────────────────┬───────────────────────────────────────┘  │
└─────────────────────┼───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Filesystem Layer                              │
│                                                                  │
│  ~/.config/agm/sessions/                                        │
│  ├── session-1/                                                 │
│  │   ├── manifest.json  ← Read by MCP server                   │
│  │   └── history.jsonl  ← NOT read (privacy)                   │
│  ├── session-2/                                                 │
│  │   └── manifest.json                                          │
│  └── ...                                                         │
└─────────────────────────────────────────────────────────────────┘
```

## Component Architecture

### 1. main.go - Entry Point

**Responsibilities**:
- Process initialization
- Configuration loading
- MCP server creation
- Tool registration
- Transport setup
- Server lifecycle management

**Key Functions**:
```go
func main()
    → loadConfig()
    → mcp.NewServer()
    → addListSessionsTool()
    → addSearchSessionsTool()
    → addGetSessionMetadataTool()
    → registerWithClaudeCode() // V2
    → server.Run(ctx, transport)
```

**Logging Strategy**:
- All logs → stderr (stdio requirement)
- Startup header with version
- Tool registration confirmation
- Error diagnostics

### 2. config.go - Configuration Management

**Responsibilities**:
- YAML config parsing
- Environment variable detection
- Smart default resolution
- Path expansion (~)

**Configuration Flow**:
```
1. Load defaults
   ↓
2. Check config file exists
   ↓
3. Parse YAML (if exists)
   ↓
4. Merge YAML with defaults
   ↓
5. Expand paths (~/)
   ↓
6. Return final Config
```

**Key Functions**:
```go
func loadConfig(configPath) → Config
func detectSessionsDir() → string
func expandHomeDir(path) → string
func registerWithClaudeCode(path) → error // V2 placeholder
```

**Configuration Struct**:
```go
type Config struct {
    Enabled          bool
    Transport        string
    Tools            []string
    AutoRegister     bool
    ClaudeConfigPath string
    SessionsDir      string
}
```

### 3. tools.go - MCP Tool Handlers

**Responsibilities**:
- Tool registration with MCP SDK
- Input validation
- Business logic orchestration
- Output formatting

**Tool Handler Pattern**:
```go
func addXTool(server *mcp.Server, cfg *Config) {
    mcp.AddTool(server, &mcp.Tool{...},
        func(ctx, req, input) → (result, output, error) {
            // 1. Validate input
            // 2. Get data (cached)
            // 3. Apply filters/search
            // 4. Transform to MCP format
            // 5. Return result
        })
}
```

**Tool Dependencies**:
- `agm_list_sessions`: cache.go → transform.go
- `agm_search_sessions`: cache.go → search logic → transform.go
- `agm_get_session_metadata`: cache.go → transform.go

**Helper Functions**:
```go
func formatJSON(v) → string
func filterSessions(sessions, status, agentType) → filtered
func searchSessionsByName(sessions, query, status) → matches
func calculateRelevance(sessionName, query) → score
```

### 4. cache.go - Session List Caching

**Responsibilities**:
- In-memory session list cache
- TTL-based expiry (5s)
- Thread-safe read/write
- Cache invalidation

**Cache Architecture**:
```go
var (
    sessionListCache []*manifest.Manifest // Cached data
    cacheTimestamp   time.Time            // Last refresh time
    cacheMutex       sync.RWMutex         // Concurrency control
)
```

**Cache Flow**:
```
Read Request
    ↓
Check cache (RLock)
    ↓
Cache hit (age <5s)?
    ├─ Yes → Return cached data
    └─ No → Release RLock
        ↓
    Acquire WLock
        ↓
    Double-check (race condition)
        ↓
    Read from disk (manifest.List)
        ↓
    Update cache + timestamp
        ↓
    Release WLock
        ↓
    Return fresh data
```

**Key Functions**:
```go
func listSessionsCached(sessionsDir) → sessions, error
func invalidateCache() // V2 feature
```

**Performance Benefits**:
- Avoids repeated disk reads
- 5s TTL balances freshness vs performance
- Double-check locking prevents thundering herd
- Read-write lock allows concurrent reads

### 5. transform.go - Data Transformation

**Responsibilities**:
- Convert manifest format to MCP format
- Timestamp formatting (RFC3339)
- Lifecycle to status mapping
- List pagination

**Key Functions**:
```go
func transformSessionsToMCP(manifests, limit) → ListSessionsOutput
func manifestToMCPMetadata(m) → MCPSessionMetadata
func mapLifecycleToStatus(lifecycle) → status
```

**Transformation Rules**:
| Manifest | MCP | Rule |
|----------|-----|------|
| `SessionID` | `id` | Direct |
| `Name` | `session_name` | Direct |
| `CreatedAt` | `created_at` | RFC3339 |
| `UpdatedAt` | `updated_at` | RFC3339 |
| `Lifecycle=""` | `status="active"` | Mapping |
| `Lifecycle="archived"` | `status="archived"` | Mapping |
| N/A | `agent_type="claude"` | Hardcode |
| `Tmux.SessionName` | `tmux_session` | Direct |

## Data Flow

### List Sessions Flow

```
Claude Code
    │
    ├─ MCP Request: agm_list_sessions
    │  {filters: {status: "active", limit: 100}}
    ▼
AGM MCP Server (tools.go)
    │
    ├─ Validate input (limit ≤1000)
    ▼
Cache Layer (cache.go)
    │
    ├─ Check cache (5s TTL)
    ├─ Cache hit? → Return cached
    └─ Cache miss → Read from disk
    ▼
Internal Manifest Library
    │
    ├─ manifest.List(sessionsDir)
    ├─ Read all manifest.json files
    └─ Return []*manifest.Manifest
    ▼
Transform Layer (transform.go)
    │
    ├─ Filter by status
    ├─ Apply limit
    ├─ Convert to MCP format
    └─ Return ListSessionsOutput
    ▼
MCP SDK
    │
    ├─ Serialize to JSON
    └─ Write to stdout
    ▼
Claude Code
```

### Search Sessions Flow

```
Claude Code
    │
    ├─ MCP Request: agm_search_sessions
    │  {query: "refactor", filters: {limit: 10}}
    ▼
AGM MCP Server (tools.go)
    │
    ├─ Validate input (query required, limit ≤50)
    ▼
Cache Layer (cache.go)
    │
    ├─ Get cached session list
    ▼
Search Logic (tools.go)
    │
    ├─ For each session:
    │   - Check status filter
    │   - Case-insensitive name match
    │   - Calculate relevance score
    ├─ Sort by relevance (desc)
    └─ Apply limit
    ▼
Transform Layer (transform.go)
    │
    ├─ Convert to MCP format
    ├─ Add relevance_score field
    └─ Return SearchSessionsOutput
    ▼
MCP SDK → Claude Code
```

### Get Session Metadata Flow

```
Claude Code
    │
    ├─ MCP Request: agm_get_session_metadata
    │  {session_id: "uuid-123"}
    ▼
AGM MCP Server (tools.go)
    │
    ├─ Validate input (session_id required)
    ▼
Cache Layer (cache.go)
    │
    ├─ Get cached session list
    ▼
Lookup Logic (tools.go)
    │
    ├─ Linear search by SessionID
    ├─ Found? → Transform to MCP
    └─ Not found? → Return error
    ▼
MCP SDK → Claude Code
```

## Concurrency Model

### Thread Safety

1. **Cache Mutex**: `sync.RWMutex` protects cache reads/writes
2. **Concurrent Reads**: Multiple tools can read cache simultaneously
3. **Exclusive Writes**: Single thread refreshes cache (write lock)
4. **Double-Check Locking**: Prevents multiple threads refreshing cache

### Goroutine Model

- **Main Goroutine**: Runs MCP server (blocking)
- **Tool Handlers**: Run in MCP SDK goroutines (concurrent)
- **No Custom Goroutines**: All concurrency handled by MCP SDK

## Error Handling Strategy

### Error Categories

1. **Configuration Errors** (Fatal)
   - Invalid YAML syntax
   - Missing sessions directory
   - Action: Log to stderr, exit process

2. **Validation Errors** (Non-Fatal)
   - Missing required fields
   - Invalid limits
   - Action: Return MCP error response

3. **System Errors** (Non-Fatal)
   - Disk read failures
   - Permission issues
   - Action: Return MCP error response, log details

4. **Not Found Errors** (Non-Fatal)
   - Session ID not found
   - Action: Return MCP error response

### Error Response Format

```go
&mcp.CallToolResult{
    Content: []mcp.Content{&mcp.TextContent{Text: "error message"}},
    IsError: true,
}
```

## Performance Architecture

### Optimization Strategies

1. **Caching**
   - Cache session list (5s TTL)
   - Avoid repeated disk I/O
   - Trade-off: Freshness vs performance

2. **Lazy Loading**
   - Only read manifest files, not history
   - Load all sessions once, filter in-memory
   - Defer full session details to future versions

3. **Efficient Data Structures**
   - Use slices for session lists (O(n) search)
   - No indexing needed for small datasets (<1000)
   - Future: Add hash map for large datasets

4. **Minimal Parsing**
   - Delegate to `internal/manifest` library
   - Reuse existing AGM parsing logic
   - No custom JSON parsing

### Performance Bottlenecks

| Component | Bottleneck | Mitigation |
|-----------|-----------|------------|
| Disk I/O | Reading all manifest files | Cache with 5s TTL |
| JSON Parsing | Unmarshaling manifests | Use internal/manifest library |
| Search | Linear search by name | Acceptable for <1000 sessions |
| Filtering | In-memory iteration | Pre-filter before transform |

### Scalability Limits

- **Sessions**: Designed for 100-1000 sessions
- **Concurrency**: MCP SDK handles concurrent requests
- **Memory**: ~1MB per 1000 sessions (manifest only)
- **Disk**: Single read per 5s (cache TTL)

## Security Architecture

### Privacy Guarantees

1. **Metadata Only**: Never access `history.jsonl` files
2. **Read-Only**: No session modification capabilities
3. **Local Process**: No network exposure
4. **Isolated**: Runs in separate process from AGM sessions

### Trust Boundaries

```
┌─────────────────────────────────────┐
│  Claude Code (Trusted)              │
│  - User's local machine             │
│  - Official Anthropic software      │
└──────────────┬──────────────────────┘
               │ stdio (local IPC)
               ▼
┌─────────────────────────────────────┐
│  AGM MCP Server (Trusted)           │
│  - User's local machine             │
│  - Open source software             │
└──────────────┬──────────────────────┘
               │ File reads
               ▼
┌─────────────────────────────────────┐
│  AGM Sessions (Protected)           │
│  - ~/.config/agm/sessions/          │
│  - User owns files                  │
│  - Only manifest.json exposed       │
└─────────────────────────────────────┘
```

### Access Control

- **File Permissions**: Respects Unix file permissions
- **Directory Traversal**: No user-controlled paths
- **Input Validation**: All inputs validated before use

## Deployment Architecture

### Build Process

```bash
cd main/agm
go build -o agm-mcp-server cmd/agm-mcp-server/*.go
```

### Installation

1. Build binary
2. Copy to `~/bin/` or system PATH
3. Register with Claude Code (manual for V1)

### Claude Code Integration

**Manual Registration** (V1):
```json
// ~/.config/claude/mcp_servers.json
{
  "agm": {
    "command": "/path/to/agm-mcp-server"
  }
}
```

**Auto-Registration** (V2):
- Server writes to `mcp_servers.json` on first run
- Updates Claude Code config atomically

### Process Lifecycle

```
Claude Code launches server
    ↓
Server initializes
    ↓
Server registers tools
    ↓
Server blocks on Run()
    ↓
[Handles requests...]
    ↓
Claude Code closes stdin
    ↓
Server exits gracefully
```

## Monitoring & Observability

### Logging

**Log Destinations**:
- stderr only (stdio requirement)
- No file-based logging in V1

**Log Levels**:
- Startup: Server version, sessions dir, tool count
- Errors: Config failures, system errors
- Debug: Not implemented in V1

**Example Logs**:
```
agm-mcp-server 1.0.0 (/path/to/binary)
Starting AGM MCP Server v1.0.0
Sessions directory: ~/.config/agm/sessions
Registered 3 tools: agm_list_sessions, agm_search_sessions, agm_get_session_metadata
Starting MCP server with stdio transport
```

### Metrics (V2)

- Query latency (p50/p99/p100)
- Cache hit rate
- Error rate by tool
- Session count over time

## Testing Architecture

### Unit Test Structure

```
cmd/agm-mcp-server/
├── main_test.go          # Config loading tests
├── tools_test.go         # Tool handler tests
├── cache_test.go         # Cache behavior tests
├── transform_test.go     # Transformation tests
└── testdata/
    └── manifests/        # Test manifest fixtures
```

### Test Categories

1. **Configuration Tests**
   - Default values
   - YAML parsing
   - Path expansion
   - Environment variable detection

2. **Tool Tests**
   - Input validation
   - Filter logic
   - Search relevance
   - Error handling

3. **Cache Tests**
   - TTL expiry
   - Thread safety
   - Invalidation

4. **Transform Tests**
   - Manifest to MCP conversion
   - Lifecycle mapping
   - Timestamp formatting

### Integration Testing

- Use MCP SDK test harness
- Mock manifest.List() for deterministic tests
- Test full request/response cycle

## Dependencies

### External Dependencies

1. **MCP Go SDK** (`github.com/modelcontextprotocol/go-sdk`)
   - Version: v1.2.0
   - Purpose: MCP protocol implementation
   - License: MIT

2. **YAML Parser** (`gopkg.in/yaml.v3`)
   - Version: v3
   - Purpose: Config file parsing
   - License: Apache 2.0

### Internal Dependencies

1. **manifest Package** (`internal/manifest`)
   - Purpose: AGM manifest parsing
   - Functions: `List()`, `Manifest` struct

### Dependency Graph

```
agm-mcp-server
├── github.com/modelcontextprotocol/go-sdk (v1.2.0)
├── gopkg.in/yaml.v3
└── internal/manifest
    └── (standard library only)
```

## Future Architecture Enhancements

### V2 Features

1. **Auto-Registration**
   - Modify Claude Code config on install
   - Atomic file writes
   - Rollback on failure

2. **Cache Invalidation**
   - Watch manifest directory for changes
   - Invalidate cache on file events
   - Reduce TTL for real-time updates

3. **Advanced Search**
   - Full-text search in metadata
   - Fuzzy matching
   - Tag-based filtering

### V3 Features

1. **WebSocket Transport**
   - Real-time session updates
   - Push notifications to Claude Code
   - Bi-directional communication

2. **Session Modification**
   - Create sessions via MCP
   - Archive/unarchive sessions
   - Update session metadata

3. **Performance Metrics**
   - Expose query metrics via MCP tool
   - Track cache hit rate
   - Monitor latency percentiles

## Design Patterns

### Patterns Used

1. **Singleton Cache**: Global cache with mutex protection
2. **Factory Pattern**: Tool registration functions
3. **Adapter Pattern**: Manifest to MCP transformation
4. **Strategy Pattern**: Different search/filter strategies

### Anti-Patterns Avoided

1. **No Global State** (except cache): All config passed via function params
2. **No Premature Optimization**: Simple linear search for V1
3. **No Overengineering**: Minimal abstractions, direct code

## Code Organization Principles

1. **Single Responsibility**: Each file has one clear purpose
2. **Minimal Dependencies**: Only essential external libraries
3. **Clear Naming**: Functions/types describe what they do
4. **Comments for Why**: Code shows how, comments explain why
5. **Test-Friendly**: Functions designed for unit testing
