---
title: Vision
version: "1.0"
status: active
date: "2026-04-05"
adr_ref: ADR-020
horizon: "6-12 months"
heuristic_style: if_then
---

# Vision

The target state for AGM: a system where multi-agent orchestration is
trustworthy by default and autonomous where appropriate.

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

If the system detects value drift --- actions that technically pass quality
gates but trend away from declared values --- then the Overseer should flag
the pattern for human review.

If a new capability is added to an agent, then its scope boundary in the
Orchestrator's dispatch table must be updated before the capability is used
in production.

If resource usage for a task exceeds the historical baseline by more than 2x,
then the Orchestrator should pause and request justification before continuing.

If an operator overrides a HITL gate, then the override must be logged with
the operator's rationale and treated as an exception, not a precedent.

If retrospective analysis reveals a recurring failure mode, then the
Meta-Orchestrator should propose a structural fix rather than adding another
ad-hoc check.
