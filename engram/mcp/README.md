# Engram MCP server

A three-tool TypeScript MCP server for local Engram retrieval, plugin discovery,
and Wayfinder status reads.

## Run

```bash
npm install
npm run build
npm test
npm start
```

Node 18 or newer is required. Configure the process with `ENGRAM_ROOT`,
`ENGRAM_CLI`, and optionally `MCP_CACHE_TTL_MS`.

| Tool | Purpose |
|---|---|
| `engram.retrieve` | Run a bounded local Engram retrieval. |
| `engram.plugins.list` | List metadata from installed Engram plugins. |
| `wayfinder.phase.status` | Read status fields from a project status file. |

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for trust boundaries and data flow and
[`SPEC.md`](SPEC.md) for executable requirements. Tool schemas in
`src/index.ts` remain authoritative.
