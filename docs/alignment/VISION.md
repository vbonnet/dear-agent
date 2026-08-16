---
title: Vision
version: "2.0"
status: active
date: "2026-07-18"
mission_ref: docs/alignment/MISSION.md
adr_ref: docs/adr/ADR-002-vroom-execution-architecture
context_ref: CONTEXT.md
horizon: "6-12 months"
---

# Vision

<!-- Last audited at: 2026-08-11 -->

MISSION.md is canonical for project purpose and ownership. The target state is
safe, auditable autonomous delivery in which VROOM makes and verifies work
decisions while AGM provides dependable session lifecycle mechanics.

## Heuristics

If an agent can complete a task within declared constraints and all quality
gates pass, then it should proceed without human intervention.

If an agent encounters ambiguity that cannot be resolved by VALUES.md or
GOALS.md, then it should escalate to the operator with a structured decision
record rather than guess.

If the decision trail for a session is incomplete or inconsistent, then the
session's outputs should be treated as unverified until the trail is repaired.

If a VROOM role's interface contract changes, then all downstream roles must
be re-validated before the change is deployed.

If repeated outcomes conflict with declared values, then VROOM should record a
DEAR finding and route a durable fix through the project tracker.

If a new capability is added to an agent, then its permission grants and role
configuration must be updated before the capability is used in production.

If resource use becomes materially higher than the task warrants, then VROOM
should preserve safety and correctness while choosing a cheaper route or
escalating with evidence.

If an operator overrides a HITL gate, then the override must be logged with
the operator's rationale and treated as an exception, not a precedent.

If retrospective analysis reveals a recurring failure mode, then VROOM should
propose a structural fix rather than adding another ad-hoc check.

See [MISSION.md](MISSION.md),
[ADR-002](../adr/ADR-002-vroom-execution-architecture.md), and
[CONTEXT.md](../../CONTEXT.md).
