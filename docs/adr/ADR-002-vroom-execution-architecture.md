# ADR-002: VROOM Execution Architecture

## Status

Accepted (2026-05-17)

**Supersedes:** `agm/docs/adr/ADR-020-vroom-architecture-overview.md` and the
per-role ADRs `agm/docs/adr/ADR-021`…`ADR-025` (Verifier/Requester/Orchestrator/
Overseer/Meta-Orchestrator). Those described an inaccurate five-role model and
were misfiled under `agm/` (VROOM is above AGM, not an AGM-internal concept).
They are stubbed with redirects to this ADR.

**Vocabulary:** Role definitions are normative in
[/CONTEXT.md](../../CONTEXT.md). This ADR records the *decision and its
trade-offs*; CONTEXT.md is the term dictionary. If the two ever disagree,
CONTEXT.md wins for definitions and this ADR wins for rationale.

## Context

VROOM is the framework under which AI agents collaborate on this repo's
engineering work. Its architecture had drifted badly in the docs:

- Six ADRs (`agm/docs/adr/ADR-020`…`025`) described VROOM as a five-role mesh
  — **V**erifier, **R**equester, **O**rchestrator, **O**verseer,
  **M**eta-Orchestrator — governed by a five-level lexicographic value
  evaluator. That is **not** the intended architecture.
- Those ADRs were filed under `agm/docs/adr/`, implying VROOM is an AGM feature.
  It is not: **VROOM is higher-level than AGM; AGM is one of the tools VROOM
  drives.**
- All six linked to `agm/docs/DEAR-PROTOCOL.md` and
  `agm/docs/orchestrator-mission.md`, **neither of which exists** (dangling
  references). ADR-020's VALUES.md link pointed outside the repo into an
  `engram-research` drafts directory.
- The alignment docs (`docs/alignment/{MISSION,VALUES,VISION,GOALS}.md`)
  inherited the wrong model via `adr_ref: ADR-020` frontmatter and references
  to a "Verifier role".

This needed a single, correct, well-placed architectural record, plus a
vocabulary source of truth (now [/CONTEXT.md](../../CONTEXT.md)).

## Decision

Adopt the **VROOM execution architecture**: a supervisory mesh that sits
**above AGM** and governs how agents do work. The model has three parts.

### 1. Per-task ownership: Primary / Secondary / Tertiary

Every task is owned by three agents — **Primary** (does it), **Secondary**
(ensures it gets done and **verifies the output**), **Tertiary** (ensures
Primary and Secondary are doing their jobs and unsticks them). Verification is a
*Secondary responsibility*, not a standing role — which is why the old
"Verifier" and "Requester" roles are removed.

### 2. Three supervisors, each in a constant loop

| Supervisor | Analogy | Owns | Secondary | Tertiary |
|------------|---------|------|-----------|----------|
| **Meta-Orchestrator** | CTO | Roadmap, prioritization, tech consistency, anti-duplication | Overseer | Orchestrator |
| **Orchestrator** | COO | Work enqueue/dequeue, worker monitoring, steady progress | Meta-Orchestrator | Overseer |
| **Overseer** | CRO | Resource usage, leak detection, session cleanup | Orchestrator | Meta-Orchestrator |

Two load-bearing invariants:

- **Mutual-unblock first.** The *first action of every loop iteration* is to
  run the supervisor-check skill against the other two supervisors and unblock
  them if needed, *before* doing the supervisor's own job. This keeps the mesh
  from deadlocking on a single stuck supervisor (historically: permission
  prompts).
- **Single roadmap authority.** Only the **Meta-Orchestrator** may add items to
  the roadmap. Everyone else proposes via a Work Order; the Meta-Orchestrator
  decides. This is the structural defense against duplicate/divergent work.

### 3. Non-supervisor agents

**Workers** (do one task, hand back for verification; Secondary = the Requester
that spawned them, Tertiary = that Requester's Secondary), **Auditors**
(periodically mine logs/DEASR retros → roadmap via Meta-Orchestrator), and
**SRE agents** (emergency-only, privileged, must first decide if there is a real
fire, cannot be spawned to bypass rules). Full definitions in
[/CONTEXT.md](../../CONTEXT.md).

### Relationship to the other frameworks

Wayfinder plans → VROOM executes → AGM is the tool VROOM drives → **DEASR**
(Diagnose → Evaluate → scAle-test → Act → Review — formerly "DEAR"; see
[ADR-024](ADR-024-deasr-push-bike-philosophy.md)) is the per-task
retrospective loop whose findings return to the roadmap through the
Meta-Orchestrator, under the **push-bike, not training wheels** design
constraint. See
[/CONTEXT.md § The Four Frameworks](../../CONTEXT.md#the-four-frameworks--and-how-they-relate).

### Decision trail

Consequential VROOM decisions are recorded to an append-only decision trail.
The concept is retained. ⚠️ The existing code seam (`pkg/vroom/vroom/topics.go`)
still encodes the *superseded* role enum (a `vroom.decision.evaluated`
"Verifier" topic, etc.). Renaming exported constants is a hard-to-reverse API
change and is **out of scope** for this docs correction; it is recorded as a
known collision in [/CONTEXT.md](../../CONTEXT.md#known-terminology-collisions)
and left for a dedicated follow-up ADR.

## Alternatives considered

1. **Keep the five-role model (status quo).** Rejected: it does not match the
   intended architecture, invents Verifier/Requester as standing roles when they
   are responsibilities/relationships, and bolts on a lexicographic value
   evaluator the system does not actually run.
2. **Single orchestrator, no peer supervisors.** Rejected: a lone supervisor has
   no one to unstick it and no separation between "what to do next" (Meta-O),
   "keep work flowing" (Orchestrator), and "is the system healthy" (Overseer).
   The mutual-unblock invariant requires ≥3 peers.
3. **Flat peer mesh (all agents equal).** Rejected: no single roadmap authority
   means duplicate/divergent work, which is the failure mode this architecture
   exists to prevent.
4. **One ADR per role (the old ADR-021…025 shape).** Rejected per the
   ADR-worthiness test (hard-to-reverse + surprising + real trade-off): the
   *single* architectural decision is "adopt this supervisory mesh". Individual
   role descriptions are vocabulary and belong in CONTEXT.md, not five ADRs that
   drift independently.

## Consequences

- VROOM docs now live at the top level (`docs/adr/`), signalling
  "above AGM". `agm/docs/adr/ADR-020`…`025` become redirect stubs (history
  preserved, dangling links removed).
- [/CONTEXT.md](../../CONTEXT.md) is the vocabulary source of truth; alignment
  docs repoint their `adr_ref` here.
- The lexicographic *value* ordering (values → goals → safety → efficiency →
  autonomy) is retained in `docs/alignment/VALUES.md` as a values-prioritization
  heuristic, but is **decoupled from role mechanics** — it is no longer "the
  Verifier enforces a five-level evaluator".
- A real code-vs-spec gap (`pkg/vroom` role enum) is now explicit and tracked
  instead of silently contradicted.
- The "VROOM" backronym is formally retired; VROOM is a proper name. A future
  maintainer may define a canonical expansion — CONTEXT.md is where it would be
  recorded.

## Cross-references

- [/CONTEXT.md](../../CONTEXT.md) — normative vocabulary
- [ADR-001: Monorepo Consolidation](ADR-001-monorepo-consolidation.md)
- [docs/alignment/VALUES.md](../alignment/VALUES.md),
  [MISSION.md](../alignment/MISSION.md),
  [VISION.md](../alignment/VISION.md),
  [GOALS.md](../alignment/GOALS.md)
- Superseded: `agm/docs/adr/ADR-020`…`ADR-025` (redirect stubs)
