# ADR-024: Overseer Role — SUPERSEDED

## Status

**Superseded (2026-05-17)** by
[`docs/adr/ADR-002: VROOM Execution Architecture`](../../../docs/adr/ADR-002-vroom-execution-architecture.md).

## Why

The Overseer still exists in the corrected VROOM architecture, but its
definition here was part of an inaccurate five-role model and was misfiled under
`agm/` (VROOM is above AGM, not an AGM feature). Corrected: the Overseer is the
**CRO / reliability supervisor** (resource usage — CPU/disk/memory/quota —
leak detection, session cleanup); its Secondary is the Orchestrator and its
Tertiary is the Meta-Orchestrator; its loop must first unblock the other two
supervisors.

Authoritative sources:

- **[docs/adr/ADR-002: VROOM Execution Architecture](../../../docs/adr/ADR-002-vroom-execution-architecture.md)**
- **[/CONTEXT.md](../../../CONTEXT.md)** — normative vocabulary

Stub retained so existing links and ADR numbering do not break.
