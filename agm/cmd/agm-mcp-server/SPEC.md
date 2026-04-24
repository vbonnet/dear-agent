# AGM MCP Server - Specification

## Overview

The AGM MCP Server is a Model Context Protocol (MCP) server that exposes AGM (AI Guided Manager) session metadata to external MCP clients like Claude Code. It enables Claude-based AI assistants to query, search, and retrieve AGM session information without accessing conversation content.

## Objectives

1. **Discoverability**: Enable Claude Code to discover and query AGM sessions
2. **Performance**: Achieve p99 <100ms response times for 1000+ sessions
3. **Privacy**: Expose only metadata, never conversation content
4. **Integration**: Seamless integration with Claude Code via MCP protocol

## Use Cases

### Primary Use Cases

1. **Session Discovery**
   - User asks Claude: "What AGM sessions do I have?"
   - Claude queries MCP server to list all sessions
   - User sees session names, IDs, and creation dates

2. **Session Search**
   - User asks Claude: "Find my session about the authentication refactor"
   - Claude searches sessions by name
   - User gets ranked results with relevance scores

3. **Session Context Retrieval**
   - User asks Claude: "What's the status of session XYZ?"
   - Claude retrieves detailed metadata for specific session
   - User sees full session details (status, timestamps, tmux info)

### Secondary Use Cases

1. **Session Filtering**
   - Filter by status (active/archived)
   - Filter by agent type (currently only Claude)
   - Limit result counts for large session lists

2. **Performance Monitoring**
   - Query large session lists efficiently
   - Cache session data to avoid repeated disk reads
   - Monitor query performance via logs

## MCP Tools

### Tool 1: agm_list_sessions

**Purpose**: List all AGM sessions with optional filters

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
  "sessions": [
    {
      "id": "uuid",
      "session_name": "string",
      "created_at": "RFC3339",
      "updated_at": "RFC3339",
      "status": "active|archived",
      "agent_type": "claude",
      "tmux_session": "string"
    }
  ],
  "total_count": 150,
  "filtered_count": 100
}
```

**Constraints**:
- `limit` maximum: 1000
- Default `limit`: 100
- Default `status`: all
- Default `agent_type`: all

**Performance Targets**:
- 100 sessions: p99 <50ms
- 500 sessions: p99 <80ms
- 1000 sessions: p99 <100ms

### Tool 2: agm_search_sessions

**Purpose**: Search AGM sessions by name with relevance ranking

**Input Schema**:
```json
{
  "query": "search string (required)",
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
      "id": "uuid",
      "session_name": "string",
      "created_at": "RFC3339",
      "updated_at": "RFC3339",
      "status": "active|archived",
      "agent_type": "claude",
      "tmux_session": "string",
      "relevance_score": 0.95
    }
  ],
  "total_matches": 5
}
```

**Relevance Scoring**:
- Exact match: 1.0
- Starts with query: 0.8
- Contains query: 0.5

**Constraints**:
- `query` is required
- `limit` maximum: 50
- Default `limit`: 10
- Case-insensitive search
- Results sorted by relevance score (descending)

### Tool 3: agm_get_session_metadata

**Purpose**: Retrieve detailed metadata for a specific session

**Input Schema**:
```json
{
  "session_id": "uuid (required)"
}
```

**Output Schema**:
```json
{
  "session": {
    "id": "uuid",
    "session_name": "string",
    "created_at": "RFC3339",
    "updated_at": "RFC3339",
    "status": "active|archived",
    "agent_type": "claude",
    "tmux_session": "string"
  }
}
```

**Constraints**:
- `session_id` is required
- Returns error if session not found
- No caching (relies on list cache)

## Data Model

### Session Metadata (MCP Format)

```go
type MCPSessionMetadata struct {
    ID             string  `json:"id"`               // Session UUID
    SessionName    string  `json:"session_name"`     // Human-readable name
    CreatedAt      string  `json:"created_at"`       // RFC3339 timestamp
    UpdatedAt      string  `json:"updated_at"`       // RFC3339 timestamp
    Status         string  `json:"status"`           // active|archived
    AgentType      string  `json:"agent_type"`       // claude (hardcoded V1)
    TmuxSession    string  `json:"tmux_session"`     // Tmux session name
    RelevanceScore float64 `json:"relevance_score"`  // Optional (search only)
}
```

### Manifest to MCP Mapping

| Manifest Field | MCP Field | Transformation |
|---------------|-----------|----------------|
| `SessionID` | `id` | Direct copy |
| `Name` | `session_name` | Direct copy |
| `CreatedAt` | `created_at` | Format as RFC3339 |
| `UpdatedAt` | `updated_at` | Format as RFC3339 |
| `Lifecycle` | `status` | Map: "" → "active", "archived" → "archived" |
| N/A | `agent_type` | Hardcode "claude" |
| `Tmux.SessionName` | `tmux_session` | Direct copy |

## Configuration

### Configuration File

**Default Path**: `~/.config/agm/mcp-server.yaml`

**Schema**:
```yaml
mcp_server:
  enabled: true
  transport: stdio
  tools:
    - agm_list_sessions
    - agm_search_sessions
    - agm_get_session_metadata
  auto_register: true
  claude_config_path: ~/.config/claude/mcp_servers.json
  sessions_dir: ~/.config/agm/sessions
```

### Configuration Precedence

1. YAML file values (highest priority)
2. Environment variables (`AGM_SESSIONS_DIR`)
3. Smart defaults (lowest priority)

### Smart Defaults

- `enabled`: `true`
- `transport`: `stdio` (only supported transport)
- `tools`: All 3 tools enabled
- `auto_register`: `true` (placeholder for V2)
- `claude_config_path`: `~/.config/claude/mcp_servers.json`
- `sessions_dir`: Auto-detected from `AGM_SESSIONS_DIR` or `~/.config/agm/sessions`

## Privacy & Security

### Exposed Data (Safe)

- Session ID (UUID)
- Session name
- Creation/update timestamps
- Status (active/archived)
- Agent type
- Tmux session name

### Protected Data (Never Exposed)

- Conversation turns
- User prompts
- Agent responses
- API keys
- Credentials
- File paths from conversation
- Any conversation content

### Security Principles

1. **Metadata Only**: Only expose session metadata from manifest files
2. **No Content Access**: Never read conversation history files
3. **Local Only**: Server runs locally, no network exposure
4. **Read-Only**: No session modification capabilities
5. **Isolation**: Runs in separate process from AGM sessions

## Performance Requirements

### Response Time Targets

| Session Count | p50 | p99 | p100 |
|--------------|-----|-----|------|
| 100 sessions | <10ms | <50ms | <100ms |
| 500 sessions | <20ms | <80ms | <150ms |
| 1000 sessions | <30ms | <100ms | <200ms |

### Optimization Strategies

1. **In-Memory Caching**: Cache session list with 5s TTL
2. **Lazy Loading**: Only load full metadata on demand
3. **Efficient Filtering**: Filter in-memory after single read
4. **Minimal Parsing**: Parse only manifest files, not history

### Cache Strategy

- **What**: Session list from `manifest.List()`
- **TTL**: 5 seconds
- **Invalidation**: On session create/update (V2 feature)
- **Concurrency**: Thread-safe with `sync.RWMutex`

## Transport Protocol

### MCP Stdio Transport

- **Protocol**: Model Context Protocol v1.2.0
- **Transport**: stdio (stdin/stdout)
- **Logging**: stderr only (critical requirement)
- **Format**: JSON-RPC 2.0

### Message Flow

1. Claude Code launches `agm-mcp-server` binary
2. Server writes header to stderr
3. Server initializes MCP server with stdio transport
4. Server registers 3 tools
5. Server blocks on `server.Run(ctx, transport)`
6. Client sends JSON-RPC requests via stdin
7. Server sends JSON-RPC responses via stdout
8. Client terminates process when done

## Error Handling

### Error Types

1. **Configuration Errors**: Invalid YAML, missing directories
2. **Session Errors**: Session not found, invalid UUID
3. **Validation Errors**: Missing required fields, invalid limits
4. **System Errors**: Disk read failures, permission issues

### Error Response Format

```json
{
  "content": [
    {
      "type": "text",
      "text": "error message"
    }
  ],
  "isError": true
}
```

### Error Messages

- Clear, actionable error messages
- Include context (e.g., session ID, limit value)
- No stack traces to clients (log to stderr)
- Graceful degradation (return empty results on non-fatal errors)

## Versioning

### Version Information

```go
var (
    Version   = "1.0.0-dev"
    GitCommit = "unknown"
    BuildDate = "unknown"
    BuiltBy   = "unknown"
)
```

- Set via ldflags at build time
- Printed to stderr on startup
- Exposed in MCP server implementation

### API Versioning

- MCP Protocol: v1.2.0 (go-sdk)
- AGM MCP Server: v1.0.0
- No breaking changes planned for v1.x

## Future Enhancements (V2+)

1. **Auto-Registration**: Automatically register with Claude Code on install
2. **Session Modification**: Create, update, archive sessions via MCP
3. **Real-Time Updates**: WebSocket transport for live session updates
4. **Advanced Search**: Full-text search in session metadata
5. **Session Grouping**: Organize sessions by project/workspace
6. **Performance Metrics**: Expose query performance metrics via MCP
7. **Multi-Agent Support**: Support non-Claude agents (GPT, Gemini)

## Testing Requirements

### Unit Tests

- Tool input validation
- Filter logic correctness
- Search relevance scoring
- Manifest to MCP transformation
- Cache behavior (hit/miss/expiry)
- Configuration loading

### Integration Tests

- MCP protocol compliance
- Claude Code integration
- End-to-end tool invocation
- Error handling

### Performance Tests

- Benchmark session list with 100/500/1000 sessions
- Measure cache hit rate
- Validate p99 latency targets
- Stress test concurrent queries

## Compliance

### MCP Specification Compliance

- Implements MCP v1.2.0 protocol
- Uses official `github.com/modelcontextprotocol/go-sdk`
- Follows stdio transport requirements
- Adheres to JSON-RPC 2.0 format

### AGM Manifest Compatibility

- Reads AGM manifest v3 format
- Compatible with existing AGM session storage
- No manifest format changes required

## References

- MCP Specification: https://modelcontextprotocol.io
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- AGM Session Manager: main/agm/
- Engram MCP Server: ./engram/main/plugins/mcp-server/
