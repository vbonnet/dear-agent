# ADR-016: Shared Operations Layer for Unified API Surfaces

**Status:** Accepted
**Date:** 2026-03-23
**Context:** AGM API unification (agm-api swarm)

> **Implementation status (2026-07-17).** Internal session archival now has a
> single durable implementation. Immediate CLI archive, bulk archive, GC, and
> the asynchronous reaper all call `ops.ArchiveSession`; the bulk command only
> selects candidates and aggregates results, while the reaper only owns its
> crash-recovery `reaping` tombstone and process-stop phase before delegating
> the final transition. `agm session archive-ui` remains a distinct operation
> and namespace per ADR-026 because it reconciles Claude UI records rather than
> AGM lifecycle storage.

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
6. **One internal archive lifecycle**: `ArchiveSession` owns archive guards,
   outcome stamping, the durable `archived` transition, harness-specific
   external archive outcomes, and post-archive cleanup. Bulk and reaper callers
   pass typed request options and preserve their surface-specific aggregate or
   asynchronous result semantics without directly mutating lifecycle storage.
7. **Reaping is preparation, not a second archive implementation**: the async
   reaper may persist `lifecycle=reaping` before stopping a pane for crash
   recovery, but only `ArchiveSession` may complete the transition to
   `lifecycle=archived`.

### Session creation amendment (2026-07-17)

Session creation is one ops-owned lifecycle, not merely a shared manifest
write. `CreateSessionWithContext` owns tmux creation/reuse, bounded Codex remote
setup, the canonical harness launch contract, manifest registration,
runtime completion ordering, and rollback. CLI and MCP adapters must
declare caller provenance. Interactive surfaces may supply one
`CreateSessionRuntime` adapter with `Launch` and `Complete` operations. The ops
module invokes those operations around registration; adapters cannot insert or
reorder lifecycle phases, and dependency ports live on `OpContext` rather than
on the creation request.

Fresh harness commands are built by `BuildHarnessLaunchCommand`. Creation,
in-place tmux startup, and fresh-session resume fallbacks use this builder so
model resolution, permission mode, persistence, telemetry, and credential
forwarding share one contract. Resume commands that target a provider-native
conversation may still add that provider's resume identifier around the shared
launch policy.

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
- Archive guard and cleanup changes now propagate uniformly to immediate,
  bulk, GC, and reaper paths; callers can no longer silently retain a copied
  lifecycle mutation.
- A failed creation removes only artifacts created by that attempt; reused tmux
  sessions and pre-existing manifest directories are preserved.
- CLI and MCP can retain different user interaction while sharing the same
  state transition order and caller provenance.
