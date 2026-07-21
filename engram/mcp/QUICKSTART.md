# Engram MCP Quick Start

## Install and verify

From `engram/mcp`:

```bash
npm ci
npm test
npm run build
```

Run `npm start` to serve MCP over stdio. The server writes protocol messages to
stdout and diagnostics to stderr.

## Configure a client

Use an absolute path to the compiled entry point:

```json
{
  "mcpServers": {
    "engram": {
      "command": "node",
      "args": ["/absolute/path/to/dear-agent/engram/mcp/dist/index.js"]
    }
  }
}
```

The `engram.retrieve` tool also requires the Engram CLI. Set `ENGRAM_CLI` when
the executable is not available as `engram` on `PATH`.

## Tool examples

Retrieve up to three memories tagged `go`:

```json
{
  "name": "engram.retrieve",
  "arguments": {"query": "error handling", "tag": "go", "limit": 3}
}
```

List plugins:

```json
{"name": "engram.plugins.list", "arguments": {}}
```

Read a Wayfinder project:

```json
{
  "name": "wayfinder.phase.status",
  "arguments": {"project": "/absolute/path/to/project"}
}
```

The Wayfinder tool returns a text content item containing JSON with exactly
these fields:

```json
{
  "project": "/absolute/path/to/project",
  "phase": "BUILD",
  "progress": "77%",
  "status": "in-progress"
}
```

The project must contain a strict canonical `WAYFINDER-STATUS.md`. Validation
errors are returned as tool text; the server does not infer state from other
files or legacy Markdown labels.

For complete request and response details, see
[ENGRAM-MCP-SERVER-API.md](./ENGRAM-MCP-SERVER-API.md).
