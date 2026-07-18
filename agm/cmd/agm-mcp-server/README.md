# AGM MCP server

Local MCP-over-stdio access to AGM session operations and Wayfinder status.

## Build and verify

```bash
make install
go test ./agm/cmd/agm-mcp-server
```

The server requires an explicit Dolt workspace through `WORKSPACE` or
`mcp_server.workspace` in `~/.config/agm/mcp-server.yaml`. Startup fails if the
selected database cannot be reached.

```yaml
mcp_server:
  enabled: true
  workspace: oss
  auto_register: true
  wayfinder_dir: /absolute/path/to/an/engram-research-worktree/wf
  a2a:
    enabled: false
    bind: 127.0.0.1
    port: 8080
```

`wayfinder_dir` must point at a writable Engram Research worktree. Do not point
it at the dear-agent source tree or a transient, branch-named checkout.

Use [`ARCHITECTURE.md`](ARCHITECTURE.md) for the current ten-tool inventory and
runtime boundaries. Use [`SPEC.md`](SPEC.md) for strict behavior requirements.
The implementation and generated MCP schemas remain authoritative.
