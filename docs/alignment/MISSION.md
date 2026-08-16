---
title: Mission
version: "2.0"
status: active
date: "2026-07-19"
adr_ref: docs/adr/ADR-002-vroom-execution-architecture
context_ref: CONTEXT.md
scope: dear-agent
---

# Mission

<!-- Last audited at: 2026-08-11 -->

`MISSION.md` is the canonical source for this project's purpose and the
VROOM/AGM ownership boundary.

## Purpose

Make autonomous multi-agent software delivery safe, auditable, and aligned
with the operator's intent. VROOM supplies the supervisory execution framework;
AGM supplies the session runtime VROOM drives.

## Ownership

VROOM owns prioritization, dispatch decisions, supervision, acceptance
criteria, and the final decision that work is acceptable. AGM owns session
lifecycle and verification mechanics: session creation, process execution,
messaging, monitoring telemetry, requested check execution, and archival.

This boundary separates decisions about **what work should happen and whether
its result is acceptable** from mechanisms that **run and observe agent
sessions**. AGM may report session or batch checks as `VERIFIED` when supplied
assertions pass. That status is evidence for VROOM; it is not an independent
acceptance decision.

## Operating principles

- Preserve safety, permission boundaries, data integrity, and operator intent.
- Record consequential decisions so another person can reconstruct the outcome.
- Verify delivered work against explicit acceptance criteria.
- Escalate when authority, evidence, or confidence is insufficient.
- Prefer the least costly approach that preserves the preceding constraints.

These principles guide judgment; they are not a runtime scoring function or an
ordered value evaluator. Executable guarantees belong in code, tests, and
enforced policies.

## Success

An operator can delegate a multi-step engineering outcome and trust VROOM to
either deliver it within declared constraints or escalate an actionable
blocker, with AGM providing an auditable session lifecycle underneath.

## Supporting documents

- [VALUES.md](VALUES.md) — non-ranked decision constraints.
- [GOALS.md](GOALS.md) — qualitative outcomes to improve.
- [ADR-002](../adr/ADR-002-vroom-execution-architecture.md) — architecture and
  trade-offs.
- [CONTEXT.md](../../CONTEXT.md) — canonical vocabulary.
