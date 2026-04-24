# ADR-019: A2A Agent Cards from Session Metadata

**Status:** Accepted
**Date:** 2026-03-24
**Context:** Standardized agent discovery using the A2A protocol

## Problem

AGM manages multiple agent sessions with rich metadata (name, purpose, harness type, tags), but this information is only accessible through AGM's proprietary CLI and API. External tools and other agent frameworks cannot discover what agents are available or what they can do.

The [A2A (Agent-to-Agent) protocol](https://github.com/a2aproject/a2a-spec) defines a standard Agent Card format for agent discovery, but AGM sessions don't expose themselves as A2A agents.

## Decision

Generate A2A Agent Cards from AGM session manifests using the official `a2aproject/a2a-go` SDK:

1. **`GenerateCard(manifest)`**: Creates an `a2a.AgentCard` with name, description, skills, and protocol version derived from session metadata
2. **`Registry`**: Manages card lifecycle on disk at `~/.agm/a2a/cards/` with CRUD operations
3. **`SyncFromManifests`**: Bulk reconciliation that adds new cards, keeps existing ones, and removes orphaned or archived sessions
4. **Skill inference**: Skills are derived from harness type, manifest tags, and session name pattern matching (e.g., a session named "code-review" gets a "code review" skill)

### Key design choices:

1. **Official SDK types**: Uses `a2a.AgentCard` and `a2a.AgentSkill` from `a2aproject/a2a-go` for protocol compliance
2. **File-based registry**: Cards stored as individual JSON files (`{session-name}.json`) for simplicity and debuggability
3. **Derived, not configured**: Skills and descriptions are inferred from existing metadata rather than requiring explicit A2A configuration
4. **Local-only**: No HTTP serving; cards are discoverable on the local filesystem only

## Alternatives Considered

1. **Manual card authoring**: Rejected -- requires users to write A2A JSON for each session; doesn't scale with dynamic session creation
2. **Database-backed registry (Dolt)**: Rejected -- cards are read-heavy and rarely updated; flat files are simpler and sufficient
3. **HTTP-first with `/.well-known/agent.json`**: Deferred -- adds HTTP server dependency; file-based is sufficient for local multi-agent scenarios
4. **Custom discovery protocol**: Rejected -- A2A is an emerging standard with SDK support; no reason to invent a proprietary format

## Consequences

- AGM sessions are discoverable by any A2A-compatible tool reading the cards directory
- No remote discovery until HTTP serving is implemented (planned as a future enhancement)
- Skill inference is heuristic-based and may not accurately represent all session capabilities
- Card files accumulate on disk and require periodic cleanup via `SyncFromManifests`
- Protocol compliance depends on the `a2a-go` SDK staying current with the A2A spec
