# ADR-022: Requester Role

**Status:** Accepted
**Date:** 2026-04-04
**Context:** Defining the Requester role in the VROOM architecture (see [ADR-020](ADR-020-vroom-architecture-overview.md))

## Problem

Work enters the system through multiple informal channels: human messages to the
orchestrator, retrospective findings, error analysis outputs, and research session
recommendations. Each channel produces work items in different formats with varying
levels of specificity. The orchestrator currently handles both goal decomposition
and task dispatch, which conflates two distinct responsibilities:

1. **What** should be done (goal → task decomposition)
2. **When and where** it should be done (scheduling and dispatch)

When the orchestrator does both, it either under-specifies tasks (workers flounder)
or over-specifies them (workers can't adapt to local state). Neither failure mode
has a clean feedback path.

## Decision

The **Requester** is the VROOM role responsible for decomposing high-level goals
into well-specified, actionable work items. It owns GOALS.md as its primary
artifact and uses Hierarchical Task Network (HTN) decomposition to break goals
into tasks that workers can execute autonomously.

### Single Responsibility

The Requester answers one question: **"What work needs to be done, and what does
'done' look like?"**

It does NOT:
- Validate outputs (Verifier)
- Schedule or dispatch tasks (Orchestrator)
- Monitor execution (Overseer)
- Govern system state (Meta-Orchestrator)

### Interface Contract

```
INPUT:
  - goal: { id, description, source, priority }
  - context: { repo_state, active_work, completed_work }

OUTPUT:
  - work_items: [{
      id, title, description,
      priority: P0-P3,
      scope: S | M | L,
      acceptance_criteria: string[],
      task_type: lint-fix | research | implementation | documentation | merge-ops,
      dependencies: work_item_id[],
      guardrails: string[]
    }]
```

### GOALS.md

GOALS.md is the Requester's primary artifact — a living document that captures
the system's current objectives and their decomposition status.

Structure:

```markdown
# Goals

## Active Goals
### G-001: <goal title>
- **Source:** human | retrospective | error-analysis | research
- **Priority:** P0-P3
- **Status:** decomposing | decomposed | in-progress | completed
- **Work Items:** WI-001, WI-002, WI-003

## Completed Goals
### G-000: <goal title>
- **Completed:** 2026-04-01
- **Work Items:** WI-000 (verified)
```

GOALS.md serves as the Define layer in the DEAR protocol for work planning.
The Orchestrator reads it to understand what to dispatch; the Verifier reads
it to understand what "done" means; the Meta-Orchestrator reads it to understand
system-level progress.

### HTN Decomposition

The Requester uses Hierarchical Task Network decomposition to break goals into
work items. HTN decomposition is a planning technique that recursively breaks
compound tasks into primitive tasks that can be executed directly.

**Decomposition rules:**

1. **Compound → Primitive:** Every goal decomposes into work items with a single
   task type (see orchestrator-mission.md § Curated Tool Subsets). A goal that
   requires both research and implementation decomposes into at least two work items.

2. **Acceptance criteria are mandatory:** Every work item has explicit, verifiable
   acceptance criteria. "Improve X" is not acceptable. "X passes test Y" is.

3. **Dependencies are explicit:** If work item B requires the output of work item A,
   the dependency is declared in the work item. The Orchestrator uses dependencies
   for dispatch ordering.

4. **Scope is bounded:** Each work item is scoped to S (< 1 hour), M (1-4 hours),
   or L (4-8 hours). Items larger than L must be further decomposed.

5. **Guardrails are inherited:** If the goal has constraints (e.g., "no breaking
   changes"), every work item inherits them as guardrails.

### Decomposition Example

```
Goal: G-005 "Implement VROOM ADRs"
├── WI-020: Write ADR-020 (documentation, S, no deps)
├── WI-021: Write ADR-021 (documentation, S, depends: WI-020)
├── WI-022: Write ADR-022 (documentation, S, depends: WI-020)
├── WI-023: Write ADR-023 (documentation, S, depends: WI-020)
├── WI-024: Write ADR-024 (documentation, S, depends: WI-020)
└── WI-025: Write ADR-025 (documentation, S, depends: WI-020)
```

### Intake Queue Integration

Decomposed work items are written to `~/.agm/intake/queue.jsonl` in the format
defined by orchestrator-mission.md § Intake Queue Consumption. The Requester
writes; the Orchestrator reads.

The Requester does NOT:
- Claim or update work item status (Orchestrator does this)
- Verify work item completion (Verifier does this)
- Monitor work item execution (Overseer does this)

### Feedback Loop

When the Verifier rejects a work item output (FAIL verdict), the feedback path is:

1. Verifier produces findings with specific gaps
2. Orchestrator routes findings back to the Requester
3. Requester evaluates whether the gap is a specification problem (refine the
   work item) or an execution problem (requeue as-is)
4. If specification problem: Requester updates acceptance criteria and creates
   a new work item revision
5. If execution problem: Requester confirms the original spec is adequate;
   Orchestrator requeues with the same spec

This implements the Resolve & Refine phase of DEAR at the goal-decomposition level.

### Research Loop Integration

The Requester is the originator in the research loop protocol (see
orchestrator-mission.md § Research Loop Tracking). When the Requester needs
investigation before decomposition:

1. Requester creates a research-type work item
2. Orchestrator dispatches to a research session
3. Research session produces findings
4. Orchestrator sends verify-request back to Requester
5. Requester evaluates findings and proceeds with decomposition

### Existing Implementation Mapping

| Requester Function | Current Implementation |
|-------------------|----------------------|
| Goal tracking | Ad-hoc (human messages, session context) |
| Work item creation | Manual queue.jsonl entries |
| HTN decomposition | Implicit in orchestrator's agentic phase |
| Acceptance criteria | Included in session prompts (inconsistently) |
| Feedback handling | Research loop protocol (partially implemented) |

## Alternatives Considered

1. **Orchestrator owns decomposition (status quo):** Conflates "what" and "when."
   The Orchestrator's context window is consumed by coordination; adding goal
   analysis degrades both functions.
2. **Human-only decomposition:** Bottleneck. The Requester can handle routine
   decomposition (error-analysis → fix tasks) autonomously, escalating only
   ambiguous goals.
3. **Flat task list (no HTN):** Loses dependency information. Workers start tasks
   before prerequisites are met, causing failures that consume error budget.

## Consequences

- GOALS.md becomes a tracked artifact alongside VALUES.md — both are inputs
  to the system's decision-making
- Work items have a formal schema enforced by the Requester before they enter
  the intake queue — garbage-in is prevented at the source
- The Orchestrator's agentic phase becomes simpler: it dispatches pre-specified
  work items rather than decomposing goals and dispatching simultaneously
- HTN decomposition adds a planning step before execution, which may slow
  initial response to urgent issues — P0 items can skip full decomposition
  with a "fast-track" flag

## Cross-References

- [ADR-020: VROOM Architecture Overview](ADR-020-vroom-architecture-overview.md)
- [ADR-021: Verifier Role](ADR-021-verifier-role.md) — validates work item outputs
- [ADR-023: Orchestrator Role](ADR-023-orchestrator-role.md) — dispatches work items
- [Orchestrator Mission § Intake Queue](../orchestrator-mission.md)
- [Orchestrator Mission § Curated Tool Subsets](../orchestrator-mission.md)
- [Orchestrator Mission § Research Loop Tracking](../orchestrator-mission.md)
- [DEAR Protocol § Define](../DEAR-PROTOCOL.md)
