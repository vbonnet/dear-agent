# Engram MCP Server

This directory contains the Node.js/TypeScript MCP server for Engram. It uses
stdio transport and exposes three read-only tools:

- `engram.retrieve` calls the Engram CLI to retrieve memories.
- `engram.plugins.list` lists plugins under the configured Engram root.
- `wayfinder.phase.status` validates `WAYFINDER-STATUS.md` as canonical
  Wayfinder schema 2.0 and returns its current state.

This package does not create or update Beads. Beads remain owned by the
canonical Beads store and CLI.

## Build and test

Requirements: Node.js 18 or newer and an `engram` executable on `PATH`.

```bash
cd engram/mcp
npm ci
npm test
npm run build
```

Start the compiled server with `npm start`. Configure another Engram CLI path
with `ENGRAM_CLI` if needed.

## MCP configuration

Build first, then configure the client with an absolute path:

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

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ENGRAM_CLI` | `engram` | Engram executable used by `engram.retrieve` |
| `ENGRAM_ROOT` | `~/.engram` | Root scanned by `engram.plugins.list` |
| `MCP_CACHE_TTL_MS` | `30000` | Tool-result cache lifetime in milliseconds |

## Source layout

- `src/index.ts` defines the MCP server and tool handlers.
- `src/wayfinder_status.ts` strictly parses Wayfinder schema 2.0.
- `src/cache.ts` provides bounded TTL caching and file-watch invalidation.
- `src/*.test.ts` contains the executable tests.

See [QUICKSTART.md](./QUICKSTART.md) for examples and
[ENGRAM-MCP-SERVER-API.md](./ENGRAM-MCP-SERVER-API.md) for exact schemas.
