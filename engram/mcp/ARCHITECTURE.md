# Engram MCP Architecture

## Boundary

This package is a small stdio adapter. It translates MCP tool calls into
read-only Engram CLI or filesystem operations; it does not own Engram memories,
Wayfinder state, plugin manifests, or Beads data.

```text
MCP client
  -> stdio server (src/index.ts)
     -> Engram CLI: engram.retrieve
     -> ~/.engram plugin manifests: engram.plugins.list
     -> WAYFINDER-STATUS.md: wayfinder.phase.status
        -> strict schema-2.0 parser (src/wayfinder_status.ts)
```

The registered names are `engram.retrieve`, `engram.plugins.list`, and
`wayfinder.phase.status`.

## Design decisions

- Tool names and schemas are declared once in `src/index.ts` beside routing.
- CLI arguments use `execFileSync` without a shell to preserve argument
  boundaries.
- Wayfinder status is parsed from one canonical file; heuristic discovery and
  legacy label matching are intentionally absent.
- Results use bounded, expiring in-memory caching. Filesystem watches provide
  earlier invalidation where available; TTL expiry remains the backstop.
- The server emits diagnostics only on stderr so stdout remains valid MCP
  transport.

## Failure behavior

Missing project files and Wayfinder parse failures are returned as text results
from the Wayfinder handler. Invalid tool names and unhandled argument errors are
returned as MCP errors with `isError: true`. Engram CLI failures are converted
to readable tool output.

## Trust boundaries

MCP arguments are untrusted. The server validates query, tag, and limit before
running the Engram CLI; resolves project paths before filesystem access; and
strictly validates the complete Wayfinder document. The client controls which
filesystem paths and Engram root are available to the process.

See [SPEC.md](./SPEC.md) for requirements and
[ENGRAM-MCP-SERVER-API.md](./ENGRAM-MCP-SERVER-API.md) for the public tool
contract.
