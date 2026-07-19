# Engram MCP server architecture

<!-- Last audited at: 2026-07-17 -->

The Engram MCP server is a small TypeScript/Node process that exposes three
local tools over MCP stdio. It retrieves memory through the Engram CLI, lists
installed plugin metadata, and reads a project's Wayfinder status file.

This package is separate from the Go AGM MCP server under
`agm/cmd/agm-mcp-server`.

## Runtime map

```text
MCP client
   -> Node >= 18 / StdioServerTransport
   -> src/index.ts tool router
      +-> engram.retrieve
      |     -> execFileSync(ENGRAM_CLI, argv, shell=false)
      +-> engram.plugins.list
      |     -> ENGRAM_ROOT/{core,user}/plugins/*/plugin.yaml
      +-> wayfinder.phase.status
            -> <project>/WAYFINDER-STATUS.md
   -> text result or MCP error result
```

`src/index.ts` is the executable owner for the tool schemas, validation, and
handlers. `src/cache.ts` owns the bounded result cache.

## Tools

| Tool | Required input | Behavior |
|---|---|---|
| `engram.retrieve` | `query` | Invokes `engram retrieve`; optional `tag` and `limit` become argv entries. |
| `engram.plugins.list` | none | Reads basic name, type, and description fields from installed plugin YAML. |
| `wayfinder.phase.status` | `project` | Reads legacy body-form phase, progress, and status fields from `WAYFINDER-STATUS.md`. |

The Wayfinder handler does not parse schema-v2 frontmatter. Missing body fields
are returned as `Unknown`; this is current behavior, not a promise that all
Wayfinder schemas are understood.

## Configuration

| Environment variable | Default | Owner |
|---|---|---|
| `ENGRAM_ROOT` | `~/.engram` | plugin discovery roots |
| `ENGRAM_CLI` | `engram` | retrieval executable |
| `MCP_CACHE_TTL_MS` | `30000` | default cache TTL in milliseconds |

The cache used by the server is capped at 200 entries. Entries expire by TTL;
plugin directories and Wayfinder status files also install non-persistent file
watchers that invalidate matching key prefixes. Retrieval results use TTL only.

## Command-execution boundary

Untrusted retrieval values are appended to an argv array and passed to
`execFileSync` with `shell: false`. They must never be interpolated into a shell
command. The child has a 30-second timeout and a 10 MiB output buffer. CLI
failures are converted to tool text rather than exposing a child-process stack.

## Protocol and error behavior

- stdout belongs to the MCP stdio transport; startup and fatal diagnostics use
  stderr.
- invalid handler input and unknown tools return `isError: true`.
- expected operational misses, such as no plugins or no Wayfinder file, return
  explanatory text.
- the process exits non-zero if server setup fails.

## Package layout

| File | Responsibility |
|---|---|
| `src/index.ts` | schemas, handlers, configuration, stdio lifecycle |
| `src/cache.ts` | TTL, capacity eviction, file-watch invalidation |
| `src/cache.test.ts` | cache behavior tests |
| `src/SPEC.md` | TypeScript implementation guardrails |
| `SPEC.md` | package behavior requirements |
| `package.json` | build, test, runtime, and dependency declarations |

Generated `dist/` output is not the source of truth and should not be committed.

## Build and verification

```bash
npm install
npm run build
npm test
```

The repository documentation-truth test also checks the finite tool inventory
and rejects the retired Python service design.

## Non-goals

This server does not embed text, import Python packages, scan arbitrary project
metadata, expose an HTTP transport, or mirror the AGM MCP lifecycle surface.
Those capabilities require their own implementation and decision record.
