# Engram

AI-powered memory and retrieval system for Claude Code sessions. Engram provides
persistent memory through hooks, ecphory-based recall, and hippocampus-style
memory consolidation.

## Components

| Directory | Purpose |
|-----------|---------|
| `cmd/engram/` | CLI entry point (Cobra-based) |
| `ecphory/` | Cue-driven memory retrieval |
| `errormemory/` | Error pattern learning and recall |
| `hippocampus/` | Memory consolidation and indexing |
| `hooks/` | Claude Code hook definitions |
| `hooks-bin/` | Compiled hook binaries |
| `internal/` | Private implementation packages |
| `mcp/` | MCP server integration |

## Installation

```bash
go install github.com/vbonnet/dear-agent/engram/cmd/engram@latest
```

## Usage

```bash
# Store a memory
engram store --type user "prefers concise responses"

# Recall memories matching a cue
engram recall "how does the user like responses?"

# Run ecphory (automatic retrieval triggered by hooks)
engram ecphory --cue "current task context"
```

## Architecture

See [cmd/engram/ARCHITECTURE.md](cmd/engram/ARCHITECTURE.md) for the full
architecture overview including the three-tier retrieval system and plugin
architecture.

## Specification

See [cmd/engram/SPEC.md](cmd/engram/SPEC.md) for functional and non-functional
requirements.

## Related

- **AGM**: Session management that integrates with engram bead tracking
- **Wayfinder**: SDLC workflow that uses engram for phase context
- **pkg/phaseengram**: Phase-to-engram file mapping used by wayfinder
