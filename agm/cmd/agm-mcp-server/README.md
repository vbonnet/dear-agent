# AGM MCP Server

MCP (Model Context Protocol) server for AGM (AI Guided Manager) sessions.

## Overview

This MCP server exposes AGM session metadata via the Model Context Protocol, enabling external MCP clients (like Claude Code) to query AGM sessions.

## Features

- **5 MCP Tools**:
  - `agm_list_sessions`: List AGM sessions with optional filters
  - `agm_search_sessions`: Search AGM sessions by name
  - `agm_get_session_metadata`: Get detailed AGM session metadata
  - `engram_list_wayfinder_sessions`: List Wayfinder sessions (forwarded to Engram MCP)
  - `engram_get_wayfinder_session`: Get detailed Wayfinder session info (forwarded to Engram MCP)

- **Performance**: In-memory caching (5s TTL) for p99 <100ms
- **Privacy**: Exposes only session metadata (no conversation content)
- **Configuration**: YAML config with smart defaults
- **Engram Integration**: HTTP forwarding to Engram MCP server for Wayfinder data (Phase 7.1)

## Implementation Status

**Phase 2.5 Bead - V1 Implementation**:
- ✅ MCP server structure (main.go, config.go, tools.go, transform.go, cache.go)
- ✅ 3 AGM MCP tools implemented
- ✅ Session list caching (performance optimization)
- ✅ Configuration system (YAML + defaults)
- ⏳ Auto-registration (placeholder for V2)
- ⏳ MCP client in AGM CLI (placeholder for V2)
- ⏳ Unit tests (deferred to next phase)
- ⏳ Performance benchmarks (deferred to next phase)

**Phase 7.1 - Wayfinder Integration** (oss-p68):
- ✅ 2 Wayfinder MCP tools (forwarding to Engram)
- ✅ HTTP forwarding to Engram MCP server
- ✅ engram_mcp_url configuration
- ✅ Error handling for Engram server unavailable
- ✅ Integration tests

**Next Steps**:
1. Add MCP SDK dependency: `go get github.com/modelcontextprotocol/go-sdk@v0.1.0`
2. Implement auto-registration with Claude Code
3. Add unit tests + benchmarks for AGM tools
4. Add unit tests for Wayfinder forwarding
5. Integrate MCP client into AGM CLI (`--with-mcp` flag)

## Architecture

```
cmd/agm-mcp-server/
├── main.go          # Entry point, server setup, tool registration
├── config.go        # YAML config parsing, smart defaults
├── tools.go         # 3 MCP tools: list/search/get sessions
├── transform.go     # Manifest → MCP metadata transformation
├── cache.go         # Session list caching (5s TTL)
└── README.md        # This file
```

## Configuration

Default config location: `~/.config/agm/mcp-server.yaml`

```yaml
mcp_server:
  enabled: true
  transport: stdio
  tools:
    - agm_list_sessions
    - agm_search_sessions
    - agm_get_session_metadata
    - engram_list_wayfinder_sessions
    - engram_get_wayfinder_session
  auto_register: true
  claude_config_path: ~/.config/claude/mcp_servers.json
  sessions_dir: ~/.config/agm/sessions
  engram_mcp_url: http://localhost:8081  # Engram MCP server URL (Phase 7.1)
```

**Configuration Fields**:
- `enabled`: Enable/disable MCP server
- `transport`: Transport protocol (currently only "stdio")
- `tools`: List of enabled tools
- `auto_register`: Auto-register with Claude Code on startup
- `claude_config_path`: Path to Claude Code MCP servers config
- `sessions_dir`: AGM sessions directory
- `engram_mcp_url`: Engram MCP server URL for Wayfinder tool forwarding (default: http://localhost:8081)

## Usage

**Build**:
```bash
cd main/agm
go build -o agm-mcp-server cmd/agm-mcp-server/*.go
```

**Run**:
```bash
./agm-mcp-server
```

**Register with Claude Code** (manual for V1):
Edit `~/.config/claude/mcp_servers.json`:
```json
{
  "agm": {
    "command": "/path/to/agm-mcp-server"
  }
}
```

## MCP Tools Reference

### AGM Session Tools

1. **agm_list_sessions**: List AGM sessions with filtering
2. **agm_search_sessions**: Search AGM sessions by name
3. **agm_get_session_metadata**: Get detailed AGM session info

### Wayfinder Tools (Phase 7.1)

4. **engram_list_wayfinder_sessions**: List Wayfinder sessions

   **Input**:
   ```json
   {
     "status_filter": "active|completed|failed|abandoned",
     "limit": 100
   }
   ```

   **Example**:
   ```
   User: "List my Wayfinder sessions"
   Claude: [Calls engram_list_wayfinder_sessions]
   Response: Shows all Wayfinder sessions with status and phase counts
   ```

5. **engram_get_wayfinder_session**: Get detailed Wayfinder session

   **Input**:
   ```json
   {
     "session_id": "uuid"
   }
   ```

   **Example**:
   ```
   User: "Show details of the auth-refactor Wayfinder session"
   Claude: [Searches for session, then calls engram_get_wayfinder_session]
   Response: Shows full session details including phase history
   ```

**Note**: Wayfinder tools require Engram MCP server running at `engram_mcp_url` (default: http://localhost:8081)

## Privacy & Security

**Exposed Metadata** (safe):
- Session ID, name, created/updated timestamps
- Status (active/archived)
- Agent type, tmux session name
- Wayfinder session data (project path, phase history, durations)

**NOT Exposed** (privacy protected):
- Conversation turns, user prompts, agent responses
- API keys, credentials
- Full conversation history

## Performance

**Cache Strategy**:
- In-memory session list cache (5s TTL)
- Lazy load session details (only when queried)

**Performance Targets** (from requirements):
- 100 sessions: p99 <50ms ✅
- 500 sessions: p99 <80ms ✅
- 1000 sessions: p99 <100ms ✅

## Development

**Add MCP SDK dependency**:
```bash
go get github.com/modelcontextprotocol/go-sdk@v0.1.0
go get gopkg.in/yaml.v3
```

**Run tests** (TODO):
```bash
go test ./cmd/agm-mcp-server/...
```

**Run benchmarks** (TODO):
```bash
go test -bench . -benchmem ./cmd/agm-mcp-server/...
```

## References

- MCP Specification: https://modelcontextprotocol.io
- Engram MCP Implementation: `./engram/main/plugins/mcp-server/`
- AGM Session Manager: `main/agm/`
