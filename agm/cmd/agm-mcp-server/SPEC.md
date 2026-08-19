# AGM MCP Server - Specification

<!-- Last audited at: 2026-08-08 -->

## Overview

The AGM MCP Server is a Model Context Protocol (MCP) server that exposes AGM (AI Guided Manager) session metadata to external MCP clients such as Claude Code, Codex, AGY, and OpenCode. It enables MCP-capable AI assistants to query, search, retrieve, and drive AGM session lifecycle operations. Every tool exposes metadata only, with one deliberate exception: `agm_get_session_output` returns captured pane content and is therefore the single conversation-content surface (see Privacy & Security).

One private production registration seam and its typed handlers own the exact
compiled tool names and schemas. `surface.Registry` is logical comparator input,
not an alternate registration source. `ops.ListOps` owns the `agm_list_ops`
catalog and must exactly project the compiled logical names. The
logical-registry-to-compiled-wire relationship and its finite compatibility
matrix are defined in `agm/internal/surface/SPEC.md` and exercised through the
actual SDK registration path.

## Objectives

1. **Discoverability**: Enable MCP clients to discover and query AGM sessions
2. **Performance**: Achieve p99 <100ms response times for 1000+ sessions
3. **Privacy**: Expose only metadata, with the single audited exception of `agm_get_session_output`, whose captured pane content may include prompts, responses, file paths, and rendered credentials. No other tool returns conversation content.
4. **Integration**: Seamless integration with supported harness clients via MCP protocol

## EARS Requirements

**MCS-01** When the MCP server exposes the create-session schema, the system shall document the selected harness default as the fallback for an omitted model.

**MCS-02** When the MCP server documents model examples, the system shall include both the supported codex-cli default alias and explicitly selectable Codex aliases.

**MCS-03** When the MCP server starts without a resolvable Dolt workspace (the WORKSPACE environment variable is unset and mcp-server.yaml declares no workspace), the system shall exit non-zero with an actionable error and shall not register any tools.

**MCS-04** When the MCP server starts and the resolved workspace's Dolt database is not reachable, the system shall exit non-zero with an error naming the workspace and shall not register any tools.

**MCS-05** When `agm_create_session` receives a valid request, the MCP adapter shall pass the request context and explicit `mcp` caller provenance to `ops.CreateSessionWithContext` rather than maintain a separate creation sequence.

**MCS-06** When MCP session creation succeeds, the result shall report `source: "mcp"` and the registered manifest shall contain the `source:mcp` provenance tag.

**MCS-07** When MCP creates a session with an initial prompt, the shared creation lifecycle shall persist required provider-native identity before the MCP runtime atomically revalidates the expected harness and current composer and delivers to that exact pane; if readiness changed during registration, MCP shall fail without sending the prompt or reporting success.

**MCS-08** When a Wayfinder MCP tool lists or retrieves a status file, the server shall accept it only after complete canonical schema 2.0 validation.

**MCS-09** When `agm_kill_session` returns success outside dry-run mode, the MCP adapter shall provide a real tmux dependency to the shared kill operation, which shall remove and verify absence of the exact resolved tmux session.

**MCS-10** When `agm_kill_session` receives a request, the MCP adapter shall propagate the request context and the explicit `force` and `confirmed_stuck` safety controls to the shared kill operation; cancellation observed before mutation shall leave tmux unchanged.

**MCS-11** When `agm_send_message` resolves a pure API manifest while the MCP operation context also carries tmux, the MCP adapter shall delegate to shared operations, which shall perform the stable-ID lifecycle, adapter-readiness, and context-aware provider transaction before any tmux probe or delivery.

**MCS-12** When `agm_create_session` receives a title that matches a non-archived session record, including when another creator commits that title concurrently, the shared creation lifecycle shall reject the request, roll back any owned launch state, and return the same duplicate-name guidance as the CLI.

**MCS-13** When provider-visible tools are registered, the system shall route production and contract tests through the same private function containing the exact compiled tool set; SDK discovery order shall not be treated as a wire contract.

**MCS-14** When list, search, get, archive, or kill input reaches a shared operation, the system shall use production-called private adapters that preserve every request value, list field mask, and mutation dry-run state.

**MCS-15** When the compiled tool surface is audited, the system shall enumerate the production SDK registration through an in-memory MCP client, preserve exact wire schema values before generic client decoding, reconcile every client-visible pagination cursor with that wire response, reject repeated non-terminal cursors before issuing another request, and reject every registry, complete property-constraint or nested array-item schema, mapping, or discovery difference not consumed by one exact compatibility record, including absent versus explicitly empty enum keywords and lossless enum member boundaries.

**MCS-16** When list, search, get, archive, or kill handlers invoke a shared operation, the system shall propagate the MCP request context separately from input-to-request adaptation.

**MCS-17** When an MCP client completes initialization, the system shall advertise the version identity of the running AGM artifact in `serverInfo.version`.

**MCS-18** When the server reports startup identity, the system shall use the same version identity that it advertises to an initialized MCP client.

**MCS-19** When a build lacks release-version metadata, the system shall expose a nonempty development fallback identity without claiming a numbered release.

**MCS-20** When `agm_list_ops` discovery is audited, the system shall advertise every compiled logical tool exactly once and no uncompiled operation.

## BDD Traceability

- Feature: `agm/test/bdd/features/mcp_parity.feature`
- Feature: `agm/test/bdd/features/harness_parity.feature`
- Test consequence: Deterministic integration test `TestMCPInitializeReportsSharedBuildVersion` observes `serverInfo.version` and the independently negotiated protocol through a real in-memory SDK initialization handshake; no new BDD feature is required because the external session BDD harness does not expose MCP initialization metadata.

## Use Cases

### Primary Use Cases

1. **Session Discovery**
   - User asks an MCP client: "What AGM sessions do I have?"
   - The client queries MCP server to list all sessions
   - User sees session names, IDs, and creation dates

2. **Session Search**
   - User asks an MCP client: "Find my session about the authentication refactor"
   - The client searches sessions by name
   - User gets ranked results with relevance scores

3. **Session Context Retrieval**
   - User asks an MCP client: "What's the status of session XYZ?"
   - The client retrieves detailed metadata for specific session
   - User sees full session details (status, timestamps, tmux info)

4. **Session Creation**
   - A client supplies a working directory, initial prompt, and optional title, harness, and model
   - The server delegates creation to the shared ops lifecycle
   - The client receives the created session ID, name, resolved harness/model, and caller provenance

### Secondary Use Cases

1. **Session Filtering**
   - Filter by status (active/archived)
   - Filter by harness
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
    "agent_type": "claude-code|codex-cli|agy|opencode-cli|pi-cli|gemini-cli|all",
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
      "harness": "claude-code",
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
      "harness": "claude-code",
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
  "identifier": "session ID, name, or UUID prefix (required)"
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
    "harness": "claude-code",
    "tmux_session": "string"
  }
}
```

**Constraints**:
- `identifier` is required
- Returns error if session not found
- No caching (relies on list cache)

### Tool 3b: agm_get_session_output

**Purpose**: Read the tail of a session's terminal output so orchestrators can collect worker results without attaching to panes

**Input Schema**:
```json
{
  "identifier": "session ID, name, or UUID prefix (required)",
  "lines": "optional trailing pane lines to capture (default 100, max 2000)"
}
```

**Output Schema**:
```json
{
  "session_id": "uuid",
  "name": "string",
  "status": "active|zombie|stopped|archived|unknown",
  "state": "string",
  "source": "live-pane|final-capture",
  "output": "string",
  "captured_at": "RFC3339"
}
```

**Constraints**:
- `identifier` is required
- Reads the live tmux pane for `active`/`zombie` sessions; falls back to the
  durable `final_output` persisted on the session record when the pane is gone
- Fallback is not guaranteed whenever `final_output` is populated. The durable
  capture describes an *earlier* completion, so it is served only when the pane
  is provably absent. If liveness cannot be confirmed — a tmux socket outage,
  permission failure, or a failed capture on a still-running pane — the
  operation returns a retryable error rather than answering with output from a
  previous task
- Returns an error when neither source has any output

### Tool 4: agm_create_session

**Purpose**: Create and register a new AGM session through the same lifecycle used by the CLI

**Input Schema**:
```json
{
  "cwd": "/absolute/project/path",
  "prompt": "initial task",
  "title": "optional-session-name",
  "harness": "claude-code|codex-cli|agy|opencode-cli|pi-cli|gemini-cli",
  "model": "optional model alias"
}
```

**Constraints**:
- `cwd` must be an existing absolute directory
- `prompt` is required
- `title`, when present, must be safe for tmux
- the resolved session name must not match any non-archived session record
- harness and model validation use the shared agent registry
- lifecycle ownership, rollback, and Codex setup remain in `internal/ops`

## Data Model

### Session Metadata (MCP Format)

```go
type MCPSessionMetadata struct {
    ID             string  `json:"id"`               // Session UUID
    SessionName    string  `json:"session_name"`     // Human-readable name
    CreatedAt      string  `json:"created_at"`       // RFC3339 timestamp
    UpdatedAt      string  `json:"updated_at"`       // RFC3339 timestamp
    Status         string  `json:"status"`           // active|archived
    Harness        string  `json:"harness"`          // claude-code, codex-cli, agy, opencode-cli, pi-cli, gemini-cli
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
| `Harness` | `harness` | Normalize to canonical harness name |
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
    - agm_get_session_output
    - agm_archive_session
    - agm_kill_session
    - agm_create_session
    - agm_send_message
    - agm_list_ops
    - engram_list_wayfinder_sessions
    - engram_get_wayfinder_session
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
- `tools`: The configured discovery, lifecycle, messaging, and Wayfinder tools
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

### Terminal Output (Exposed Only Through `agm_get_session_output`)

`agm_get_session_output` is a deliberate, bounded exception to the rules below,
not an oversight. Rendered terminal panes routinely contain prompts, model
responses, and file paths, so the guarantees in this section are scoped rather
than absolute. The exception's boundary:

- **Opt-in per call.** Output is returned only when a caller names a session and
  explicitly invokes `agm_get_session_output`. No metadata, list, or search tool
  returns it — `agm_get_session_metadata` exposes only `final_output_at`, the
  timestamp proving a capture exists, never its contents.
- **Rendered pane only.** The source is the tmux pane (live) or the ≤16 KiB tail
  captured at completion. Conversation history files, transcripts, and provider
  API state are never read — principle 2 below is unchanged.
- **Why it exists.** Without it an orchestrator cannot collect a worker's result
  without attaching to its pane, which is what forced result-polling (ce-0zng9).

### Protected Data (Never Exposed)

Except through `agm_get_session_output` as scoped above, where the value appears
in rendered terminal output:

- Conversation turns
- User prompts
- Agent responses
- File paths from conversation
- Any conversation content
- API keys and credentials, to the extent a harness rendered them into its pane

The server never *reads* credential stores, config, or history files. But it
cannot sanitize a pane: anything a harness printed to the terminal — including a
secret it echoed — is inside the rendered capture. Treat
`agm_get_session_output` results with the same care as the terminal itself.

### Security Principles

1. **Metadata, Lifecycle, and Explicitly Requested Output**: Expose session
   metadata and lifecycle operations freely; expose terminal output only through
   the dedicated opt-in tool above
2. **No Content Access**: Never read conversation history files
3. **Local Only**: Server runs locally, no network exposure
4. **Shared Mutation Boundary**: Lifecycle mutations delegate to `internal/ops` validation and rollback
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

- **Protocol**: Model Context Protocol version negotiated by the pinned SDK
- **Transport**: stdio (stdin/stdout)
- **Logging**: stderr only (critical requirement)
- **Format**: JSON-RPC 2.0

### Message Flow

1. An MCP client launches `agm-mcp-server` binary
2. Server writes header to stderr
3. Server initializes MCP server with stdio transport
4. Server registers the configured discovery, lifecycle, messaging, and Wayfinder tools
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

## Implementation Identity and Protocol

The server's implementation identity is the running AGM artifact version. The
build system supplies release provenance through the repository's shared
version package; an unstamped developer build uses that package's nonempty
development fallback. The first process header, structured startup log, and
MCP `serverInfo.version` all expose this same identity.

Implementation identity is not the MCP wire protocol version. The pinned MCP
SDK independently negotiates protocol support with each client.

## Future Enhancements (V2+)

1. **Auto-Registration**: Automatically register with supported MCP clients on install
2. **Session Modification**: Update sessions via MCP
3. **Real-Time Updates**: WebSocket transport for live session updates
4. **Advanced Search**: Full-text search in session metadata
5. **Session Grouping**: Organize sessions by project/workspace
6. **Performance Metrics**: Expose query performance metrics via MCP
7. **Expanded Harness Support**: Add parity coverage for future harnesses

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
- Supported MCP client integration
- End-to-end tool invocation
- Error handling
- Compiled registration, semantic schema, request mapping, and discovery
  compatibility against the logical operation registry

### Performance Tests

- Benchmark session list with 100/500/1000 sessions
- Measure cache hit rate
- Validate p99 latency targets
- Stress test concurrent queries

## Compliance

### MCP Specification Compliance

- Negotiates supported MCP protocol versions through the pinned SDK
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
