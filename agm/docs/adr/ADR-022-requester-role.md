# ADR-022: Requester Role — SUPERSEDED

## Status

**Superseded (2026-05-17)** by
[`docs/adr/ADR-002: VROOM Execution Architecture`](../../../docs/adr/ADR-002-vroom-execution-architecture.md).

## Why

There is **no standing "Requester" role** in the corrected VROOM architecture.
"Requester" is a *relationship*: the agent that spawned a given Worker (which
becomes that Worker's Secondary). It is not a supervisor, and goal decomposition
is not a separate role. This ADR belonged to the inaccurate five-role model and
was misfiled under `agm/`.

Authoritative sources:

- **[docs/adr/ADR-002: VROOM Execution Architecture](../../../docs/adr/ADR-002-vroom-execution-architecture.md)**
- **[/CONTEXT.md](../../../CONTEXT.md)** — normative vocabulary

Stub retained so existing links and ADR numbering do not break.
