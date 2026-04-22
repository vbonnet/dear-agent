# ADR-016: Shared Operations Layer for Unified API Surfaces

**Status:** Accepted
**Date:** 2026-03-23
**Context:** AGM API unification (agm-api swarm)

## Problem

AGM exposes three API surfaces — CLI (Cobra), MCP (JSON-RPC), and Skills (markdown) — but they had independent implementations with different behavior:
- CLI commands contained business logic inline in Cobra RunE handlers
- MCP server read from deprecated YAML manifest files instead of Dolt
- Skills called CLI commands but had no structured error handling
- Error messages were human-oriented, not useful for AI agents

## Decision

Introduce `internal/ops/` as a shared operations layer that all three surfaces call:

```
CLI (Cobra)    →  internal/ops  →  Dolt Storage
MCP (JSON-RPC) →  internal/ops  →  Dolt Storage
Skills (.md)   →  CLI --json    →  internal/ops  →  Dolt Storage
```

### Key design choices:

1. **OpContext for dependency injection**: Storage, tmux, config, and output preferences passed via `OpContext` struct
2. **RFC 7807 errors**: All errors return `OpError` with stable codes (AGM-001+), actionable `suggestions`, and echoed `parameters`
3. **Field masks**: `ApplyFieldMask()` filters JSON output to requested fields, reducing token consumption
4. **Typed request/result structs**: Every operation has `*Request` input and `*Result` output, both JSON-serializable
5. **Skills use CLI with `--output json`**: Rather than importing Go directly, skills shell out to `agm --output json` and parse structured output

## Alternatives Considered

1. **gRPC service layer**: Rejected — over-engineered for a single-user CLI tool
2. **Refactor CLI commands only**: Rejected — doesn't help MCP or Skills
3. **MCP-first with CLI as thin client**: Rejected — CLI needs to work without MCP server running

## Consequences

- All three surfaces guarantee identical behavior for the same operation
- New operations only need one implementation (in ops), then thin adapters in each surface
- Error codes are stable contracts that agents can match on programmatically
- MCP server no longer needs separate YAML manifest reading code
- Slight overhead: each MCP tool call creates a new Dolt connection (acceptable for low-frequency tool use)
