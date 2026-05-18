# ADR-025: Meta-Orchestrator Role — SUPERSEDED

## Status

**Superseded (2026-05-17)** by
[`docs/adr/ADR-002: VROOM Execution Architecture`](../../../docs/adr/ADR-002-vroom-execution-architecture.md).

## Why

The Meta-Orchestrator still exists in the corrected VROOM architecture, but its
definition here was part of an inaccurate five-role model and was misfiled under
`agm/` (VROOM is above AGM, not an AGM feature). Corrected: the
Meta-Orchestrator is the **CTO supervisor** (roadmap, prioritization, technology
consistency, anti-duplication) and is the **only agent allowed to add items to
the roadmap**; its Secondary is the Overseer and its Tertiary is the
Orchestrator; its loop must first unblock the other two supervisors.

Authoritative sources:

- **[docs/adr/ADR-002: VROOM Execution Architecture](../../../docs/adr/ADR-002-vroom-execution-architecture.md)**
- **[/CONTEXT.md](../../../CONTEXT.md)** — normative vocabulary

The operational mission doc [meta-orchestrator-mission.md](../meta-orchestrator-mission.md)
is retained and updated to match the corrected model.

Stub retained so existing links and ADR numbering do not break.
