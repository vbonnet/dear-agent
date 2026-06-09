---
title: Mission
version: "1.0"
status: active
date: "2026-04-05"
adr_ref: docs/adr/ADR-002-vroom-execution-architecture
context_ref: CONTEXT.md
scope: dear-agent
supervisors:
  meta_orchestrator: "roadmap, prioritization, tech consistency (CTO); sole roadmap-add authority"
  orchestrator: "work enqueue/dequeue, worker monitoring, steady progress (COO)"
  overseer: "resource usage, leak detection, session cleanup (CRO)"
task_ownership: "Primary (does it) / Secondary (verifies it) / Tertiary (unsticks them)"
---

# Mission

This project exists to make autonomous multi-agent orchestration **safe,
auditable, and aligned** with its operator's intent.

## Purpose

Provide a supervisory **execution framework — VROOM** (see
[CONTEXT.md](../../CONTEXT.md) and
[docs/adr/ADR-002](../adr/ADR-002-vroom-execution-architecture.md)) — where AI
agents collaborate on software engineering tasks under structured governance.
VROOM drives **AGM** (a tool) to run agent sessions. Every consequential
decision is evaluated against declared values, traced in an append-only log,
and subject to human-in-the-loop gates when confidence is insufficient.

## Scope

AGM governs the lifecycle of agent sessions: creation, dispatch, monitoring,
verification, and archival. It does not own the work products themselves; it owns
the process by which agents produce and validate those products.

## Operating Principle

Prefer to ask a human rather than violate a higher-priority concern. Autonomy is
valuable only after values compliance, goal alignment, safety invariants, and
resource efficiency are satisfied --- in that lexicographic order.

## Success Criterion

An operator can delegate a multi-step engineering task to AGM and trust that the
system will either complete it within declared constraints or escalate clearly,
with a full decision trail explaining why.
