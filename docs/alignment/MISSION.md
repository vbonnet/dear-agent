---
title: Mission
version: "1.0"
status: active
date: "2026-04-05"
adr_ref: ADR-020
scope: ai-tools
role_mapping:
  verifier: "VALUES.md compliance"
  requester: "GOALS.md decomposition"
  orchestrator: "session dispatch"
  overseer: "anomaly detection"
  meta_orchestrator: "state machine governance"
---

# Mission

AGM exists to make autonomous multi-agent orchestration **safe, auditable, and
aligned** with its operator's intent.

## Purpose

Provide a supervisory framework (VROOM) where AI agents collaborate on software
engineering tasks under structured governance. Every consequential decision is
evaluated against declared values, traced in an append-only log, and subject to
human-in-the-loop gates when confidence is insufficient.

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
