# AGM MCP server architecture

<!-- Last audited at: 2026-07-17 -->

`agm-mcp-server` is a local control surface over shared AGM operations and
Wayfinder status files. Its primary transport is MCP over stdio. An optional
read-only A2A HTTP endpoint publishes agent cards; it is not an alternate MCP
transport.

## Startup boundary

```text
load ~/.config/agm/mcp-server.yaml
    -> resolve WORKSPACE (environment wins over config)
    -> verify the selected Dolt database is reachable
    -> register ten MCP tools
    -> install optional gateway middleware
    -> optionally update the Claude Code MCP config
    -> run stdio MCP transport
    -> optionally run A2A card endpoint on loopback
```

The server fails before tool registration when no explicit workspace can be
resolved or its Dolt database is unreachable. This prevents a protocol-valid but
nonfunctional tool surface.

## Registered tools

Tool registration in `main.go` is authoritative.

| Tool | Operation owner |
|---|---|
| `agm_list_sessions` | `ops.ListSessions` |
| `agm_search_sessions` | `ops.SearchSessions` |
| `agm_get_session_metadata` | `ops.GetSession` |
| `agm_archive_session` | `ops.ArchiveSession` |
| `agm_kill_session` | `ops.KillSession` |
| `agm_create_session` | `ops.CreateSessionWithContext` |
| `agm_send_message` | `ops.SendMessage` |
| `agm_list_ops` | `ops.ListOps` |
| `engram_list_wayfinder_sessions` | local Wayfinder status reader |
| `engram_get_wayfinder_session` | local Wayfinder status reader |

The first eight tools expose AGM operations. The last two read
`WAYFINDER-STATUS.md` frontmatter from the configured directory. They do not
proxy through the separate Engram MCP process.

## Request flow

```text
MCP request
   -> gateway middleware when enabled
   -> typed handler in tools.go
   -> new operation context
      -> fresh Dolt adapter per request
      -> tmux dependency for create/send operations
   -> shared internal/ops function
   -> JSON result in MCP text content
```

There is no in-process session-manifest cache. Storage freshness, connection
behavior, validation, dry-run semantics, and lifecycle rollback belong to the
shared operation and storage layers.

Known `ops.OpError` values are returned as RFC 7807 JSON in an MCP error result.
Other errors are returned as error text. Tool errors do not crash the server.

## Mutation boundary

Archive and kill accept `dry_run`. Create requires a working directory and
prompt and records MCP caller provenance. Send requires a target session and
message. Create and send operation contexts include tmux; read operations do
not.

The server exposes session metadata and lifecycle operations, not conversation
history. Any future content-reading tool requires a separate privacy and
authorization decision.

## Wayfinder boundary

`wayfinder.go` lists directories under `wayfinder_dir`, reads frontmatter from
`WAYFINDER-STATUS.md`, and supports current plus selected legacy field names.
Single-session reads reject path traversal characters. The default directory is
`~/src/engram-research/wf`, but deployments should configure the actual evidence
checkout rather than rely on that development-oriented default.

## Optional surfaces

### Gateway

`internal/gateway` middleware is installed when its separate configuration is
enabled. `--no-gateway` bypasses it for controlled diagnostics.

### A2A agent cards

When enabled, an HTTP server publishes an aggregate card and per-session cards
at `/.well-known/...`. It reads active manifests through the Dolt storage
boundary and accepts only GET requests. The server binds the configured
`mcp_server.a2a.bind` address verbatim, so a non-loopback value exposes this
surface to that network. The flag port overrides configuration; an enabled
endpoint without a port uses 8080.

### Auto-registration

When enabled, the server atomically updates the configured Claude Code MCP JSON
entry with its current executable path while preserving unrelated entries and
existing per-server fields.

## Process and transport invariants

- JSON-RPC is written only through the stdio transport; diagnostics go to
  stderr.
- stdin EOF and parent-process reparenting both stop the server.
- the optional A2A server shuts down with the MCP process.
- W3C trace context is propagated where downstream calls support it.
- workspace selection is explicit and verified before the surface becomes
  visible.

## Source owners and verification

| Concern | Owner |
|---|---|
| Lifecycle and registrations | `main.go` |
| Tool schemas and handlers | `tools.go` |
| Configuration and auto-registration | `config.go` |
| Wayfinder file reads | `wayfinder.go` |
| A2A cards | `a2a_handler.go` |
| Shared AGM behavior | `agm/internal/ops` |

Run `go test ./agm/cmd/agm-mcp-server` and the relevant `internal/ops` tests.
Strict requirements are in [`SPEC.md`](SPEC.md); the durable surface decision is
indexed in [`adr/README.md`](adr/README.md).
