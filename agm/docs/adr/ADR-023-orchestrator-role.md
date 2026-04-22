# ADR-023: Orchestrator Role

**Status:** Accepted
**Date:** 2026-04-04
**Context:** Defining the Orchestrator role in the VROOM architecture (see [ADR-020](ADR-020-vroom-architecture-overview.md))

## Problem

The orchestrator session is the most complex component in AGM, handling session
lifecycle management, task dispatch, permission management, error budgets, and
partial goal decomposition. Its mission document (orchestrator-mission.md) is
1,300+ lines covering responsibilities that span multiple VROOM roles. Without
a formal role boundary, the orchestrator tends to accumulate responsibilities
("if it's coordination, the orchestrator does it"), leading to context window
pressure and compaction fragility.

## Decision

The **Orchestrator** is the VROOM role responsible for session lifecycle
management and task dispatch. It is the execution engine — it takes well-specified
work items from the Requester and manages their execution through worker sessions.

### Single Responsibility

The Orchestrator answers one question: **"How should this work be executed?"**

It does NOT:
- Define what work to do or decompose goals (Requester)
- Validate outputs against values (Verifier)
- Detect cross-session anomalies (Overseer)
- Make system-level governance decisions (Meta-Orchestrator)

### Interface Contract

```
INPUT:
  - work_items: [{ id, title, description, priority, scope, task_type,
                    acceptance_criteria, dependencies, guardrails }]
  - system_state: { active_sessions, error_budget, capacity }

OUTPUT:
  - session_actions: [{
      action: launch | nudge | terminate | archive,
      session_id: string,
      work_item_id: string,
      tool_profile: string,
      rationale: string
    }]
  - state_update: orchestrator-state.json delta
```

### Session Lifecycle State Machine

Every worker session managed by the Orchestrator follows this state machine:

```
                    ┌─────────┐
                    │ queued  │  (work item claimed, session not yet launched)
                    └────┬────┘
                         │ launch
                         ▼
                    ┌─────────┐
              ┌─────│ active  │─────┐
              │     └────┬────┘     │
              │          │          │
         stuck│     done │     fail │
              │          │          │
              ▼          ▼          ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ stalled  │ │completing│ │ failed   │
        └────┬─────┘ └────┬─────┘ └────┬─────┘
             │            │            │
        nudge│     verify │    requeue │
             │            │            │
             ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ active   │ │ verified │ │ queued   │
        │(retry)   │ │          │ │(retry)   │
        └──────────┘ └────┬─────┘ └──────────┘
                          │
                     archive
                          │
                          ▼
                    ┌──────────┐
                    │ archived │
                    └──────────┘
```

State transitions are logged to `orchestrator-state.json` with timestamps.
The Verifier gates the `completing → verified` transition. The Overseer may
trigger `active → stalled` detection.

### Task Dispatch

The Orchestrator dispatches work items using these rules:

1. **Capacity check:** Never exceed `throttle_config.max_parallel` sessions
   (adjusted by error budget enforcement level)
2. **Dependency check:** Only dispatch work items whose dependencies are in
   `verified` or `archived` state
3. **Priority ordering:** P0 > P1 > P2 > P3, then oldest first within priority
4. **Tool profile assignment:** Match `task_type` to the appropriate tool profile
   (see orchestrator-mission.md § Curated Tool Subsets)
5. **Session naming:** `{work_item_slug}` (unique, descriptive, no conflicts)

### Scan Loop (Blueprint Pattern)

The Orchestrator's scan loop follows the deterministic/agentic separation
documented in orchestrator-mission.md:

```
SCAN CYCLE START
│
├─ DETERMINISTIC PHASE (no LLM cost)
│  ├─ Read orchestrator-state.json
│  ├─ List managed sessions (agm session list)
│  ├─ Check session states (capture-pane)
│  ├─ Read intake queue for new items
│  ├─ Evaluate error budget
│  ├─ Check session lifecycle states
│  └─ Write state checkpoint
│
├─ AGENTIC PHASE (LLM reasoning)
│  ├─ Evaluate session progress (active → stalled?)
│  ├─ Compose worker instructions for queued items
│  ├─ Decide intervention for flagged sessions
│  └─ Emit reasoning traces
│
└─ DETERMINISTIC PHASE (execute decisions)
   ├─ Launch sessions (agm session new)
   ├─ Send messages (agm send msg)
   ├─ Approve permission prompts
   └─ Write final state
```

### Error Budget Integration

The Orchestrator owns error budget tracking (SRE pattern, see
orchestrator-mission.md § SRE Error Budget Tracking). Error budgets are
evaluated in the deterministic phase and affect dispatch capacity:

| Enforcement Level | Max Parallel Sessions |
|-------------------|----------------------|
| normal (< 50%) | 4 |
| throttled (50-99%) | 2 |
| frozen (≥ 100%) | 0 (escalate to Meta-Orchestrator) |

### Command Allowlist

The Orchestrator runs ONLY commands from the documented allowlist
(orchestrator-mission.md § Command Allowlist). This is enforced at three levels:

1. **Soft:** This ADR and the mission document (LLM reads and complies)
2. **Medium:** CLAUDE.md conditional rules (loaded on session start)
3. **Hard:** PreToolUse hook `pretool-orchestrator-guard` (blocks mechanically)

### State Persistence

`orchestrator-state.json` is the Orchestrator's persistent state file, updated
atomically at the end of each scan cycle. It contains:

- `managed_sessions`: Current session states and metadata
- `intake_tracking`: Queue consumption state
- `error_budget`: SLO counters and enforcement level
- `permission_patterns`: Pattern classification state
- `research_loops`: Research→implementation cycle state

### Existing Implementation Mapping

| Orchestrator Function | Current Implementation |
|----------------------|----------------------|
| Session lifecycle | `agm session new/list/archive`, scan loop |
| Task dispatch | Agentic phase of scan loop |
| Error budget | `orchestrator-state.json` error_budget key |
| State persistence | `orchestrator-state.json` |
| Command restriction | `pretool-orchestrator-guard` hook |
| Tool profiles | Session instruction prompts |

## Alternatives Considered

1. **Orchestrator owns everything (status quo):** Works but accumulates
   responsibilities. Each new protocol (error budgets, research loops,
   permission patterns) adds to the mission doc. VROOM decomposes these
   responsibilities cleanly.
2. **Daemon-only orchestrator (astrocyte):** The deterministic phase maps
   to astrocyte; the agentic phase requires LLM judgment. Hybrid is correct.
3. **Event-driven (no scan loop):** AGM's file-based architecture doesn't
   support event notification natively. Polling via scan loop is the pragmatic
   choice until an event bus is built (see [ADR-009](ADR-009-eventbus-multi-agent-integration.md)).

## Consequences

- The Orchestrator's mission doc can be simplified by extracting goal
  decomposition to the Requester and anomaly detection to the Overseer
- Session lifecycle state machine is formalized — all valid transitions are
  enumerated, invalid transitions are architectural violations
- Error budget enforcement becomes a dispatch constraint, not a separate
  protocol — it's integrated into the Orchestrator's capacity check
- The Orchestrator's context window is protected: it coordinates, it does
  not investigate, implement, or decompose

## Cross-References

- [ADR-020: VROOM Architecture Overview](ADR-020-vroom-architecture-overview.md)
- [ADR-022: Requester Role](ADR-022-requester-role.md) — provides work items
- [ADR-021: Verifier Role](ADR-021-verifier-role.md) — gates archive transition
- [ADR-024: Overseer Role](ADR-024-overseer-role.md) — flags stalled sessions
- [ADR-025: Meta-Orchestrator Role](ADR-025-meta-orchestrator-role.md) — governs Orchestrator
- [ADR-009: EventBus Multi-Agent Integration](ADR-009-eventbus-multi-agent-integration.md)
- [Orchestrator Mission](../orchestrator-mission.md)
- [DEAR Protocol](../DEAR-PROTOCOL.md)
