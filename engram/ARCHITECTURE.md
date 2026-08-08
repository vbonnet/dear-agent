# Engram — Architecture

<!-- Last audited at: NEEDS-AUDIT -->

## System Context

Engram runs as a CLI tool and hook handler within Claude Code sessions. It
reads and writes memory files on the local filesystem, integrating with Claude
Code's hook system for automatic retrieval.

```
┌──────────────┐     hooks      ┌─────────┐     file I/O    ┌──────────────┐
│  Claude Code │ ──────────────>│  Engram  │ ──────────────> │ Memory Store │
│  (session)   │ <──────────────│  CLI     │ <────────────── │ (~/.claude/  │
└──────────────┘   injected     └─────────┘                  │  projects/)  │
                   context          │                        └──────────────┘
                                    │ MCP
                                    v
                              ┌───────────┐
                              │ MCP Server│
                              └───────────┘
```

## Component Architecture

### CLI Layer (`cmd/engram/`)
- Cobra-based command tree: `store`, `recall`, `ecphory`, `consolidate`
- Structured error handling (ADR-002)
- Formatted output with table and JSON modes (ADR-003)

### Ecphory Engine (`ecphory/`)
- Three-tier retrieval: exact match → keyword → semantic (ADR-004)
- Cue analysis and memory scoring
- Context window budget management

### Error Memory (`errormemory/`)
- Pattern extraction from tool call failures
- Similarity matching against stored error patterns
- Resolution suggestion based on past fixes

### Hippocampus (`hippocampus/`)
- Memory consolidation pipeline
- Duplicate detection and merging
- Staleness scoring and pruning

### Hooks (`hooks/`, `hooks-bin/`)
- Pre-compiled hook binaries for performance
- Hook definitions for Claude Code integration
- Event-driven memory operations

### MCP Server (`mcp/`)
- Model Context Protocol server implementation
- Exposes memory operations to MCP-compatible clients

### MCP Executable Identity (`cmd/engram-mcp/`)
- The startup log and MCP server identity both read `pkg/version.Version`.
- The `build-engram-mcp` target stamps that owner through the repository-wide
  `BUILD_STAMP_FLAGS` profile.
- An artifact-level test builds the executable with a non-default stamp and
  verifies the startup log and raw MCP initialize response agree.
- The separately released `engram` CLI retains its own version contract.

### Internal (`internal/`)
- Shared utilities and types
- Memory file parsing and serialization

## Knowledge Layers

Engram stores knowledge in two distinct layers (see ADR-009):

### Document Layer (`internal/document/`)
- Stateless, immutable, **versioned** knowledge blobs (specs, architecture,
  research, reference, ADRs) — trusted as-is, never mutated in place
- `Store` interface with an append-only versioning contract (`Put` appends a
  new version; no in-place update); `FSStore` reference implementation stores
  one JSON file per version under `{root}/{namespace…}/{id}/v{N}.json`
- Content-hashed (SHA-256) versions; path-containment validation (ADR-006)

### Memory Layer (`internal/consolidation/`, `hippocampus/`)
- Mutable, extracted facts distilled from session history
- Consolidation pipeline: decay, merge, prune, importance scoring
- Pluggable `Provider` interface (ADR-007) with in-place updates

This separation keeps consolidation lifecycle rules off canonical documents and
sharpens recall precision. The corpus-callosum schema registers both a
`document` and a `memory_trace` schema (component schema `1.1.0`).

## Key Decisions

See `cmd/engram/ADR-INDEX.md` for the full list of Architecture Decision Records.
