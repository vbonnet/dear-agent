# ADR-020: VROOM Architecture Overview — SUPERSEDED

## Status

**Superseded (2026-05-17)** by
[`docs/adr/ADR-002: VROOM Execution Architecture`](../../../docs/adr/ADR-002-vroom-execution-architecture.md).

## Why

This ADR described an inaccurate five-role model (Verifier / Requester /
Orchestrator / Overseer / Meta-Orchestrator with a lexicographic value
evaluator) and was **misfiled under `agm/`**. VROOM is *higher-level than AGM* —
AGM is one of the tools VROOM drives, not its owner. The ADR also linked to
`agm/docs/DEAR-PROTOCOL.md` and `agm/docs/orchestrator-mission.md`, neither of
which exists.

The correct architecture (three supervisors — Meta-Orchestrator / Orchestrator /
Overseer — plus per-task Primary/Secondary/Tertiary ownership, Workers,
Auditors, and SRE agents) is recorded in:

- **[docs/adr/ADR-002: VROOM Execution Architecture](../../../docs/adr/ADR-002-vroom-execution-architecture.md)** — the decision and its trade-offs
- **[/CONTEXT.md](../../../CONTEXT.md)** — normative vocabulary (single source of truth for terms)

This stub is retained so existing links and ADR numbering do not break.
