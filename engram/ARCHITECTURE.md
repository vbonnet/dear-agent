# Engram — Architecture

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

### Internal (`internal/`)
- Shared utilities and types
- Memory file parsing and serialization

## Key Decisions

See `cmd/engram/ADR-INDEX.md` for the full list of Architecture Decision Records.
