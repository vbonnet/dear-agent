# ADR-020: VROOM Architecture Overview

**Status:** Accepted
**Date:** 2026-04-04
**Context:** Formalizing the five-role supervisory architecture for autonomous agent orchestration

## Problem

AGM's multi-agent orchestration has evolved organically into a set of roles documented
across mission docs, protocols, and operational runbooks (orchestrator-mission.md,
meta-orchestrator-mission.md, DEAR-PROTOCOL.md). The roles, their boundaries, and their
interactions are implicit — scattered across documents without a unifying architectural
model. This makes it hard to reason about the system as a whole, onboard new contributors,
or verify that the architecture is complete.

We need a named, bounded architecture that:
1. Enumerates every supervisory role and its single responsibility
2. Defines how roles interact (interfaces, not implementations)
3. Establishes a decision evaluation framework that prevents value drift
4. Produces an auditable decision trail for every consequential action

## Decision

Adopt the **VROOM** architecture: five named roles that collectively govern autonomous
agent work. VROOM stands for **V**erifier, **R**equester, **O**rchestrator, **O**verseer,
**M**eta-Orchestrator.

### The Five Roles

| Role | Single Responsibility | Primary Artifact |
|------|----------------------|-----------------|
| **Verifier** | Validate that outputs conform to values and invariants | VALUES.md, quality gates |
| **Requester** | Decompose goals into actionable work items | GOALS.md, intake queue |
| **Orchestrator** | Manage session lifecycle and dispatch tasks | orchestrator-state.json |
| **Overseer** | Detect anomalies and escalate across sessions | escalation log |
| **Meta-Orchestrator** | Govern the system state machine and HITL gates | system state, HITL decisions |

### Role Interaction Model

```
                    ┌──────────────────┐
                    │ Meta-Orchestrator │
                    │  (state machine)  │
                    └────────┬─────────┘
                             │ governs
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
      ┌───────────┐  ┌──────────────┐  ┌──────────┐
      │ Requester │  │ Orchestrator │  │ Overseer │
      │  (goals)  │  │  (dispatch)  │  │ (watch)  │
      └─────┬─────┘  └──────┬───────┘  └────┬─────┘
            │               │               │
            │  work items   │  sessions     │  alerts
            └───────────────┤               │
                            ▼               │
                    ┌──────────────┐        │
                    │   Workers    │◄───────┘
                    └──────┬───────┘  monitors
                           │
                           ▼
                    ┌──────────────┐
                    │   Verifier   │
                    │  (validate)  │
                    └──────────────┘
```

### Lexicographic Evaluation Order

When a decision involves competing concerns, VROOM evaluates them in strict
lexicographic (priority) order. Higher-priority concerns are satisfied first;
lower-priority concerns are optimized only within the feasible set left by
higher priorities.

**Priority order (highest first):**

1. **Values compliance** — Does the action conform to VALUES.md? (Verifier domain)
2. **Goal alignment** — Does the action advance a declared goal? (Requester domain)
3. **Safety invariants** — Does the action preserve system invariants? (Overseer domain)
4. **Resource efficiency** — Does the action use resources well? (Orchestrator domain)
5. **Autonomy** — Can the action proceed without human intervention? (Meta-Orchestrator domain)

This means: an efficient action that violates values is always rejected. A safe action
that doesn't advance any goal is deprioritized. Autonomy is optimized last — the system
prefers to ask a human rather than violate any higher-priority concern.

### Decision Trail

Every consequential decision in VROOM produces a **decision record** — a structured
trace that captures what was decided, by which role, against which evaluation criteria,
and why. This extends the existing reasoning trace log (ADR-020 cross-ref:
orchestrator-mission.md § SRE Explicit Reasoning Traces).

Decision record fields:

| Field | Description |
|-------|-------------|
| `role` | Which VROOM role made the decision |
| `decision_type` | Category: dispatch, escalation, rejection, approval, gate |
| `evaluation` | Lexicographic evaluation result (which priority levels were checked) |
| `outcome` | What was decided |
| `rationale` | Why, referencing specific priority level that determined the outcome |

Decision records are appended to `~/.agm/logs/decision-trail.jsonl`, one JSON object
per line. They are append-only and never retroactively edited.

### Relationship to DEAR Protocol

VROOM roles map to DEAR phases:

| DEAR Phase | Primary VROOM Role(s) |
|------------|----------------------|
| **Define** | Requester (goals), Meta-Orchestrator (system spec) |
| **Enforce** | Verifier (deterministic checks), Orchestrator (quality gates) |
| **Audit** | Overseer (async anomaly detection), Verifier (post-execution) |
| **Resolve & Refine** | Meta-Orchestrator (root cause → system update) |

### Relationship to Existing Infrastructure

VROOM names and formalizes roles that already exist in the codebase:

| VROOM Role | Existing Implementation |
|------------|------------------------|
| Verifier | `/engram:bow`, quality gates, `/agm:audit-completion` |
| Requester | `~/.agm/intake/queue.jsonl`, work request protocol |
| Orchestrator | `orchestrator-mission.md`, `orchestrator-state.json`, scan loop |
| Overseer | Pattern detection playbook, permission refine loop |
| Meta-Orchestrator | `meta-orchestrator-mission.md`, supervisory observation loop |

## Alternatives Considered

1. **Three-role model (Orchestrator, Workers, Verifier):** Too coarse — conflates goal
   decomposition with dispatch, and conflates anomaly detection with governance.
2. **Flat peer model (all roles equal):** No clear escalation path. Requires consensus
   protocols that add latency without adding value in a single-operator system.
3. **Unnamed roles (status quo):** Roles exist but without a shared vocabulary, making
   cross-referencing and architectural reasoning harder.

## Consequences

- All future ADRs, mission docs, and protocol specs use VROOM role names consistently
- Each role has a single ADR (ADR-021 through ADR-025) defining its interface contract
- The lexicographic evaluation order is a system invariant — changing priority order
  requires a new ADR with explicit justification
- Decision trail logging adds operational overhead but provides the audit capability
  needed for autonomous operation
- VROOM is an architectural model, not an implementation spec — roles may be implemented
  as LLM sessions, daemons (astrocyte), hooks, or combinations thereof

## Cross-References

- [ADR-021: Verifier Role](ADR-021-verifier-role.md)
- [ADR-022: Requester Role](ADR-022-requester-role.md)
- [ADR-023: Orchestrator Role](ADR-023-orchestrator-role.md)
- [ADR-024: Overseer Role](ADR-024-overseer-role.md)
- [ADR-025: Meta-Orchestrator Role](ADR-025-meta-orchestrator-role.md)
- [DEAR Protocol](../DEAR-PROTOCOL.md)
- [Orchestrator Mission](../orchestrator-mission.md)
- [Meta-Orchestrator Mission](../meta-orchestrator-mission.md)
- [VALUES.md](../../../../engram-research/projects/governance-restoration-2025-11/drafts/VALUES.md)
