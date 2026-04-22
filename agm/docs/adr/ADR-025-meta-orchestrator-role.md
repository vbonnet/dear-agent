# ADR-025: Meta-Orchestrator Role

**Status:** Accepted
**Date:** 2026-04-04
**Context:** Defining the Meta-Orchestrator role in the VROOM architecture (see [ADR-020](ADR-020-vroom-architecture-overview.md))

## Problem

The meta-orchestrator session currently handles supervisory observation, pattern
detection, root cause analysis, orchestrator coaching, and human escalation. Its
mission document describes a broad mandate ("make itself unnecessary") without
crisp boundaries on what decisions it can and cannot make autonomously. This
creates two risks:

1. **Authority creep:** The meta-orchestrator accumulates governance power
   without explicit HITL (Human-In-The-Loop) gates, potentially making
   consequential system changes without human awareness.
2. **Role blur:** Pattern detection (now Overseer) and output validation
   (now Verifier) are mixed into the meta-orchestrator's observation loop,
   making its core responsibility unclear.

## Decision

The **Meta-Orchestrator** is the VROOM role responsible for top-level system
governance. It operates the system state machine, enforces HITL gates for
consequential decisions, drives root cause analysis, and manages the feedback
loop that makes the system permanently better after every failure.

### Single Responsibility

The Meta-Orchestrator answers one question: **"What system-level changes should
be made, and which require human approval?"**

It does NOT:
- Monitor individual sessions (Overseer)
- Validate specific outputs (Verifier)
- Decompose goals into tasks (Requester)
- Dispatch or manage worker sessions (Orchestrator)

### Interface Contract

```
INPUT:
  - overseer_alerts: [{ id, severity, category, evidence, recommended_action }]
  - system_state: { orchestrator_status, error_budget, active_goals, queue_depth }
  - decision_trail: recent entries from decision-trail.jsonl
  - verifier_results: recent verification outcomes

OUTPUT:
  - directives: [{
      type: launch_fix_session | update_instructions | change_policy |
            escalate_to_human | adjust_capacity | trigger_compaction,
      target: string,
      rationale: string,
      hitl_required: bool,
      hitl_gate: string  // which gate, if hitl_required
    }]
  - state_transition: { from_state, to_state, trigger }
```

### System State Machine

The Meta-Orchestrator governs the top-level system state. All VROOM roles
operate within the constraints of the current system state.

```
                    ┌────────────────┐
                    │  initializing  │
                    └───────┬────────┘
                            │ all roles ready
                            ▼
                    ┌────────────────┐
         ┌─────────│    active      │──────────┐
         │         └───────┬────────┘          │
         │                 │                   │
    queue empty     frozen budget        P0 alert
    + no workers           │                   │
         │                 ▼                   ▼
         │         ┌────────────────┐  ┌────────────────┐
         │         │   degraded    │  │  emergency     │
         │         └───────┬────────┘  └───────┬────────┘
         │                 │                   │
         │           budget reset        human resolves
         │                 │                   │
         │                 ▼                   ▼
         │         ┌────────────────┐  ┌────────────────┐
         │         │    active     │  │    active      │
         │         └────────────────┘  └────────────────┘
         │
         ▼
    ┌────────────────┐
    │     idle       │
    └───────┬────────┘
            │ new work arrives
            ▼
    ┌────────────────┐
    │    active      │
    └────────────────┘
```

**State definitions:**

| State | Meaning | Constraints |
|-------|---------|-------------|
| `initializing` | System starting, roles loading | No dispatch, no monitoring |
| `active` | Normal operation | Full dispatch, all roles operating |
| `degraded` | Error budget frozen or major component impaired | No new dispatch, existing sessions monitored |
| `emergency` | P0 alert, safety issue, or system invariant violated | All dispatch stopped, HITL required |
| `idle` | No work, no active sessions | Loops stopped, awaiting new input |

### HITL Gates

Certain decisions are too consequential for autonomous execution. The
Meta-Orchestrator enforces HITL gates — decision points where human approval
is required before proceeding.

| Gate | Trigger | What Requires Approval |
|------|---------|----------------------|
| **G1: Policy Change** | Directive to change system policy (e.g., SLO targets, tool profiles) | The specific policy change and its rationale |
| **G2: Emergency Stop** | P0 alert from Overseer | Whether to stop all work or isolate the problem |
| **G3: Values Update** | Proposed change to VALUES.md or evaluation order | The change and its impact analysis |
| **G4: Capacity Override** | Proposal to override error budget enforcement | The override and its time-bound scope |
| **G5: Architecture Change** | New ADR or architectural decision | The decision and its consequences |
| **G6: Third Compaction** | System reaching 3rd compaction limit | Whether to continue or restart |

### HITL Gate Protocol

```
1. Meta-Orchestrator identifies decision requiring HITL gate
2. Decision is logged to decision-trail.jsonl with hitl_required: true
3. Alert sent to human via configured channel (agm send msg to human session)
4. System blocks on the gated decision (other operations continue)
5. Human approves, modifies, or rejects
6. Resolution logged to decision trail
7. If approved: Meta-Orchestrator executes the directive
8. If rejected: Meta-Orchestrator logs rationale and seeks alternative
```

### Root Cause Analysis

When the Overseer escalates a P1 alert, the Meta-Orchestrator's primary
response is root cause analysis — tracing from symptom to systemic fix:

```
Symptom (Overseer alert)
  │
  ├─ Immediate cause: What directly triggered it?
  │
  ├─ Instruction cause: Which instruction/prompt led to the behavior?
  │
  ├─ Tool cause: Which tool design enables the failure mode?
  │
  └─ Architecture cause: Which architectural gap allows this class of error?
```

Each level produces a different fix type:

| Cause Level | Fix Type | Example |
|-------------|----------|---------|
| Immediate | Workaround | Send nudge to stuck session |
| Instruction | Prompt update | Add guardrail to worker prompt |
| Tool | Code fix | Add validation to `agm send msg` |
| Architecture | ADR/Protocol update | New VROOM protocol for the failure class |

The Meta-Orchestrator directs the Orchestrator to launch fix sessions for
instruction, tool, and architecture-level causes. It does not implement fixes.

### Resolve & Refine Ownership

The Meta-Orchestrator is the primary owner of the DEAR Protocol's Resolve &
Refine phase at the system level. While individual roles handle local R&R
(e.g., Verifier refines its checks, Orchestrator refines its dispatch), the
Meta-Orchestrator handles systemic R&R — changes that affect multiple roles
or the architecture itself.

Systemic R&R actions:
- Update mission documents (Define layer)
- Add new hooks or enforcement rules (Enforce layer)
- Modify monitoring rules (Audit layer, delegated to Overseer)
- Create new ADRs for architectural changes

### Loop Coordination

The Meta-Orchestrator coordinates the start/stop of all VROOM role loops:

| System State | Orchestrator Loop | Overseer Loop | Meta-Orchestrator Loop |
|-------------|-------------------|---------------|----------------------|
| `active` | Running | Running | Running |
| `degraded` | Running (throttled) | Running | Running |
| `emergency` | Stopped | Running | Running |
| `idle` | Stopped | Stopped | Stopped |
| `initializing` | Starting | Starting | Running |

### Self-Obsolescence

The Meta-Orchestrator's stated goal is to make itself unnecessary
(meta-orchestrator-mission.md). In VROOM terms, this means:

1. Root cause fixes should reduce the frequency of P1 alerts over time
2. Deterministic rules should absorb agentic decisions as patterns stabilize
3. Astrocyte should absorb the Overseer's detection rules
4. HITL gates should decrease as the system proves reliability

Success metric: the Meta-Orchestrator's intervention rate decreases
monotonically over time. If it increases, the system is not improving.

### Existing Implementation Mapping

| Meta-Orchestrator Function | Current Implementation |
|---------------------------|----------------------|
| System state management | Implicit (meta-orchestrator session context) |
| HITL gates | Ad-hoc human escalation |
| Root cause analysis | Meta-orchestrator pattern detection + fix sessions |
| Loop coordination | Loop coordination protocol in mission docs |
| Self-obsolescence tracking | "Gaps Closed" table in orchestrator-mission.md |

## Alternatives Considered

1. **No Meta-Orchestrator (Orchestrator self-governs):** The Orchestrator
   cannot govern itself — it needs external oversight for compaction recovery,
   emergency stops, and policy changes. Self-governance creates blind spots.
2. **Human-only governance:** Bottleneck for routine decisions. The
   Meta-Orchestrator handles routine governance autonomously, escalating only
   through HITL gates.
3. **Consensus model (all roles vote):** Adds latency and complexity without
   clear benefit in a single-operator system. Hierarchical governance with
   HITL gates is simpler and sufficient.

## Consequences

- System state is explicitly tracked — the current state of the system is
  always known and logged, not inferred from individual role states
- HITL gates formalize the boundary between autonomous and human-supervised
  operation — the system cannot make certain decisions without human approval
- Root cause analysis is structured (4-level trace) rather than ad-hoc
- The Meta-Orchestrator's scope is narrowed from "everything the orchestrator
  can't do" to "system governance and HITL gates" — pattern detection moves
  to the Overseer, output validation moves to the Verifier
- Self-obsolescence is measurable: intervention rate should decrease over time

## Cross-References

- [ADR-020: VROOM Architecture Overview](ADR-020-vroom-architecture-overview.md)
- [ADR-021: Verifier Role](ADR-021-verifier-role.md)
- [ADR-022: Requester Role](ADR-022-requester-role.md)
- [ADR-023: Orchestrator Role](ADR-023-orchestrator-role.md)
- [ADR-024: Overseer Role](ADR-024-overseer-role.md) — feeds alerts to Meta-Orchestrator
- [Meta-Orchestrator Mission](../meta-orchestrator-mission.md)
- [Orchestrator Mission](../orchestrator-mission.md)
- [DEAR Protocol § Resolve & Refine](../DEAR-PROTOCOL.md)
