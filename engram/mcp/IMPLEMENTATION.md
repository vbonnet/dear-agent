# Engram MCP Implementation

The active implementation is the TypeScript service in `src/`. Generated
JavaScript and declarations are written to ignored `dist/` files by `npm run
build`.

## Request path

1. `src/index.ts` starts an MCP `Server` over `StdioServerTransport`.
2. `tools/list` publishes the three tool definitions.
3. `tools/call` validates and routes the tool name and arguments.
4. The handler returns one MCP text content item. Unhandled validation or
   routing failures set `isError: true`.

## Tool implementations

- `engram.retrieve` invokes `ENGRAM_CLI` with `execFileSync` and an argument
  array. No shell interprets user input. Query results are cached by query,
  optional tag, and limit.
- `engram.plugins.list` scans `core/plugins` and `user/plugins` beneath
  `ENGRAM_ROOT`. A directory is listed only when it contains `plugin.yaml`.
- `wayfinder.phase.status` resolves the requested project, reads
  `WAYFINDER-STATUS.md`, and calls `parseWayfinderStatus`. It returns project,
  phase, progress, and status.

`src/wayfinder_status.ts` is a strict schema-2.0 reader. It rejects malformed
frontmatter, unknown fields, invalid enum values or timestamps, inconsistent
lifecycle state, invalid roadmap references, duplicate waypoint history, and
attempts to skip mandatory waypoints.

## Cache behavior

`src/cache.ts` provides a bounded in-memory TTL cache. The server uses a
30-second default and a 200-entry limit. Plugin directories and Wayfinder status
files are watched so relevant changes invalidate cached results before expiry
when the platform delivers a watch event.

## Verification

```bash
npm test
npm run build
```

The tests exercise cache expiry/invalidation and strict Wayfinder parsing. The
TypeScript build verifies the server and handlers against their declared types.

The sibling Go MCP implementation under `engram/cmd/engram-mcp` is a separate
binary with its own contract. Documentation in this directory describes only
this TypeScript package.
