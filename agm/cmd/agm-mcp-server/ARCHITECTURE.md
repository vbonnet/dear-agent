<!-- Last audited at: 2026-06-12 ce-dn72 -->

# AGM MCP Server — Architecture

## Overview

The AGM MCP Server (`agm-mcp-server`) is a lightweight MCP (Model Context
Protocol) server that bridges Claude Code (and other MCP clients) with AGM
session management and Wayfinder project tracking. It runs as a single
stdio process and provides eleven tools across three domains:

- **AGM session tools** — list, search, get, create, message, archive, and kill sessions
- **Schema tool** — introspect available ops at runtime
- **Wayfinder tools** — list and get Wayfinder sessions from the local filesystem

The server delegates all AGM session logic to `agm/internal/ops` (which owns
the Dolt storage layer) and reads Wayfinder data directly from
`WAYFINDER-STATUS.md` files on disk.

The running artifact's implementation identity comes from the shared
`pkg/version` package and is exposed consistently in the process header,
startup log, and MCP initialization response. This build identity is distinct
from the wire protocol version, which the MCP SDK negotiates independently.

## Source Files

| File | Responsibility |
|------|---------------|
| `main.go` | Entry point, flag parsing, server lifecycle, A2A HTTP |
| `config.go` | YAML config loading, smart defaults, auto-registration |
| `tools.go` | MCP tool registration and handler functions |
| `wayfinder.go` | Filesystem-based Wayfinder session reads |
| `a2a_handler.go` | Agent-to-Agent HTTP endpoint |

## Registered Tools

| Tool name | Source | Description |
|-----------|--------|-------------|
| `agm_list_sessions` | `tools.go` | List AGM sessions with status/type/limit filters |
| `agm_search_sessions` | `tools.go` | Search sessions by partial name match |
| `agm_get_session_metadata` | `tools.go` | Full metadata for a session by ID/name (never includes captured output) |
| `agm_get_session_output` | `tools.go` | Tail of a session's terminal output: live pane, or the durable final capture after completion |
| `agm_archive_session` | `tools.go` | Mark a session archived (supports dry-run) |
| `agm_kill_session` | `tools.go` | Kill and verify the exact tmux session (supports dry-run and explicit safety confirmation) |
| `agm_create_session` | `tools.go` | Create an AGM-managed session |
| `agm_send_message` | `tools.go` | Send a message to an AGM-managed session |
| `agm_list_ops` | `tools.go` | List all available ops (schema discovery) |
| `engram_list_wayfinder_sessions` | `tools.go` + `wayfinder.go` | List Wayfinder sessions from `wf/` directory |
| `engram_get_wayfinder_session` | `tools.go` + `wayfinder.go` | Get full frontmatter for one Wayfinder session |

## Configuration (`Config` struct)

```go
type Config struct {
    Enabled          bool      // Whether the MCP server is enabled
    Transport        string    // Transport type (currently only "stdio")
    Tools            []string  // Tool allowlist (empty = all registered)
    AutoRegister     bool      // Auto-write to Claude Code MCP config
    ClaudeConfigPath string    // Path to Claude Code MCP JSON config
    SessionsDir      string    // AGM sessions directory
    EngramMCPURL     string    // Reserved for future HTTP transport
    WayfinderDir     string    // Path to engram-research wf/ directory
    A2A              A2AConfig // Agent-to-Agent HTTP endpoint config
}
```

Default config file: `~/.config/agm/mcp-server.yaml` (created if absent).
Default `WayfinderDir`: `~/src/engram-research/wf/`.

## Data Flow

### AGM session tools

```
Claude Code → MCP request
    ↓
tools.go handler
    ↓
newMCPOpContext()        → opens Dolt DB via agm/internal/dolt
    ↓
ops.ListSessions() / ops.SearchSessions() / ops.GetSession() /
ops.ArchiveSession() / ops.KillSession() / ops.CreateSessionWithContext() /
ops.SendMessage() / ops.ListOps()
    ↓
mcpSuccess(result)       → JSON-encoded CallToolResult
    ↓
Claude Code ← MCP response
```

Each AGM tool handler creates a fresh Dolt connection per request
(`newMCPOpContext`) and defers `adapter.Close()`. There is no in-process
caching — Dolt handles its own connection pooling and reads.

### Wayfinder tools

```
Claude Code → MCP request
    ↓
tools.go handler
    ↓
wayfinder.go: listWayfinderSessions() / getWayfinderSessionDetail()
    ↓
os.ReadDir(wayfinderDir)
    ↓
for each session dir: statusread.Parse(WAYFINDER-STATUS.md bytes)
    ↓
mcpSuccess(result)       → JSON-encoded CallToolResult
    ↓
Claude Code ← MCP response
```

Wayfinder tools read `WAYFINDER-STATUS.md` files directly from the local
`engram-research/wf/` directory. No HTTP proxy or network call is involved.
Only a complete, valid canonical schema 2.0 status is accepted. The shared
`statusread` adapter validates the exact bytes before the MCP adapter extracts
the fields needed for list and detail responses.

## Key Invariants

- **Path traversal protection**: `getWayfinderSessionDetail` rejects session
  IDs containing `/`, `\`, `.`.
- **Dry-run support**: `ArchiveSession` and `KillSession` respect
  `opCtx.DryRun` when the `dry_run` input field is set.
- **Tool errors are non-fatal**: All handler errors return an MCP error
  response (with `IsError: true`) rather than crashing the server.
- **OpError → RFC 7807 JSON**: `mcpError()` unwraps `*ops.OpError` and
  returns the structured JSON body rather than a plain string.

## Optional Features

### MCP Gateway

If `~/.config/agm/gateway.yaml` exists and `enabled: true`, the gateway
middleware is installed via `agm/internal/gateway` before the server runs.
The `--no-gateway` flag bypasses it. Gateway config defaults are permissive.

### A2A HTTP Server

The Agent-to-Agent HTTP endpoint is enabled via `a2a.enabled: true` in the
config or `--a2a-port <n>`. It binds to `127.0.0.1:<port>` (default 8080)
and handles A2A protocol payloads via `a2a_handler.go`. It runs in a
goroutine alongside the stdio transport and shuts down when the MCP server
exits.

### Process Lifecycle & Orphan Prevention

Two mechanisms prevent orphaned server processes when the parent Claude Code
session exits:

1. **stdin EOF (belt 1)** — the go-sdk `StdioTransport` goroutine detects EOF
   on stdin and cancels the root context. This handles normal termination and
   clean process-exit.

2. **Parent PID poll (belt 2)** — a goroutine polling every 5 seconds calls
   `os.Getppid()` and compares it to the PID captured at startup. If the
   parent is reparented to PID 1 (the OOM-kill scenario where stdin EOF may
   not arrive before the OS reparents the child), it calls `stop()`. Both
   goroutines carry `defer recover()` per the goroutine-recover structural
   health policy.

### Auto-registration

When `auto_register: true` (default), the server writes or updates an `agm`
entry in the Claude Code MCP config file on startup. The write is atomic
(tmp + rename) and idempotent — it is a no-op when the `command` field
already matches the current executable.

## Dependencies

| Dependency | Purpose |
|-----------|---------|
| `github.com/modelcontextprotocol/go-sdk/mcp` | MCP protocol SDK (stdio transport, tool registration) |
| `gopkg.in/yaml.v3` | Config file and Wayfinder frontmatter parsing |
| `agm/internal/ops` | AGM session business logic |
| `agm/internal/dolt` | Dolt storage adapter |
| `agm/internal/gateway` | MCP gateway middleware |
| `go.opentelemetry.io/otel` | W3C trace context injection for downstream propagation |
