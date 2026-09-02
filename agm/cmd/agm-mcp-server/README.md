# AGM MCP Server

MCP (Model Context Protocol) server for AGM session management and Wayfinder project tracking.

## Overview

The AGM MCP Server bridges Claude Code (and other MCP clients) with AGM session data
and local Wayfinder project files. It runs as a stdio process and registers 14 tools
across 4 domains.

## Tools

### AGM session tools

| Tool | Description |
|------|-------------|
| `agm_list_sessions` | List sessions with status/type/limit filters |
| `agm_search_sessions` | Search sessions by partial name match |
| `agm_get_session_metadata` | Full session metadata by ID or name |
| `agm_get_session_output` | Read live or durably captured terminal output |
| `agm_archive_session` | Mark a session archived (dry-run supported) |
| `agm_kill_session` | Kill the exact tmux session (`dry_run`, `force`, and active-session `confirmed_stuck` supported) |
| `agm_create_session` | Create an AGM-managed session |
| `agm_send_message` | Send a message to an AGM-managed session |

### Schema tool

| Tool | Description |
|------|-------------|
| `agm_list_ops` | List available ops (schema discovery) |

### Dispatch routing tools

| Tool | Description |
|------|-------------|
| `agm_get_quota_status` | Read recorded provider quota status for routing decisions |
| `agm_get_completion_relay_target` | Read the live completion-relay target |
| `agm_set_completion_relay_target` | Point completion relay at a live Dispatch session |

### Wayfinder tools

| Tool | Description |
|------|-------------|
| `engram_list_wayfinder_sessions` | List Wayfinder sessions from `wf/` directory |
| `engram_get_wayfinder_session` | Get full frontmatter for one session by directory name |

Wayfinder tools read `WAYFINDER-STATUS.md` files directly from
`~/src/engram-research/wf/` (configurable). No external server required.

## Configuration

Default config: `~/.config/agm/mcp-server.yaml`

```yaml
mcp_server:
  enabled: true
  transport: stdio
  auto_register: true
  sessions_dir: ~/.config/agm/sessions
  wayfinder_dir: ~/src/engram-research/wf/
  # engram_mcp_url: reserved for future HTTP transport
  a2a:
    enabled: false
    port: 8080
    bind: 127.0.0.1
```

## Usage

**Build and install**:
```bash
make install   # installs agm-mcp-server to GOBIN
```

**Run directly** (auto-registers with Claude Code on first run):
```bash
agm-mcp-server
```

**Run tests**:
```bash
go test ./agm/cmd/agm-mcp-server/...
```

## Files

```
agm/cmd/agm-mcp-server/
├── main.go            # Entry point, tool registration, A2A HTTP server
├── config.go          # YAML config loading, auto-registration
├── tools.go           # MCP tool handlers (delegates to agm/internal/ops)
├── wayfinder.go       # Filesystem-based Wayfinder session reads
└── a2a_handler.go     # Agent-to-Agent HTTP endpoint
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full design.

## Privacy

Most tools expose only session metadata. `agm_get_session_output` is the
deliberate exception: its captured terminal output may contain prompts,
responses, file paths, or rendered credentials and must be treated as
sensitive. No tool reads API-key storage directly.
