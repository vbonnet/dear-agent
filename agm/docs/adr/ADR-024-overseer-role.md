# ADR-024: Overseer Role

**Status:** Accepted
**Date:** 2026-04-04
**Context:** Defining the Overseer role in the VROOM architecture (see [ADR-020](ADR-020-vroom-architecture-overview.md))

## Problem

The current architecture detects problems at two levels: the Orchestrator monitors
individual sessions, and the Meta-Orchestrator detects cross-session patterns. But
there is no dedicated watchdog role that continuously monitors for anomalies,
safety violations, and systemic drift — independently of the roles that are
executing or governing work. The Orchestrator has a conflict of interest (it
launched the sessions it monitors), and the Meta-Orchestrator operates at too high
a level for real-time anomaly detection.

We need an independent, async watchdog that:
1. Monitors all sessions without being part of the execution chain
2. Detects anomalies that individual session monitoring misses
3. Has a clear escalation ladder for different severity levels
4. Cannot be overridden by the roles it monitors

## Decision

The **Overseer** is the VROOM role responsible for asynchronous anomaly detection
and escalation. It is an independent watchdog — it monitors but never executes,
dispatches, or governs. Its only output is alerts routed through a severity-based
escalation ladder.

### Single Responsibility

The Overseer answers one question: **"Is anything going wrong that other roles
haven't noticed?"**

It does NOT:
- Execute or dispatch work (Orchestrator)
- Decompose goals (Requester)
- Validate specific outputs (Verifier)
- Make governance decisions (Meta-Orchestrator)

### Interface Contract

```
INPUT:
  - session_states: [{ session_id, state, last_output, duration, cycle_count }]
  - system_metrics: { error_budget, active_sessions, queue_depth }
  - decision_trail: recent entries from decision-trail.jsonl
  - permission_log: recent entries from permission-patterns.jsonl

OUTPUT:
  - alerts: [{
      id: string,
      severity: P0_EMERGENCY | P1_URGENT | P2_WATCH | P3_INFO,
      category: string,
      description: string,
      evidence: string[],
      recommended_action: string,
      escalation_target: verifier | orchestrator | meta_orchestrator | human
    }]
```

### Anomaly Detection Categories

| Category | What It Detects | Example |
|----------|----------------|---------|
| **Stall** | Session with no progress across N scan cycles | Worker at permission prompt for 10+ minutes |
| **Loop** | Session repeating the same action/error | Worker retrying failed `git push` 5 times |
| **Drift** | Behavior diverging from task spec | Implementation session doing research |
| **Escape** | Safety bypass or quality gate circumvention | Worker using `--no-verify` flag |
| **Cascade** | Multiple sessions failing from shared root cause | 3 workers hitting same broken dependency |
| **Budget** | Error budget approaching or exceeding threshold | Completion rate SLO at 80% consumed |
| **Permission** | Repeated permission prompts indicating misconfiguration | Same `chmod` prompt across 4 sessions |

### Escalation Ladder

Alerts are routed based on severity. Higher severity = faster response, higher
escalation target.

```
P3_INFO ──────────► Log only (decision-trail.jsonl)
                    No action required. Informational.

P2_WATCH ─────────► Orchestrator
                    "Be aware of this. Monitor on next cycle."
                    Example: session running longer than expected

P1_URGENT ────────► Meta-Orchestrator
                    "Intervene. This needs a decision."
                    Example: error budget at 90%, cascade detected

P0_EMERGENCY ─────► Human (HITL gate)
                    "Stop. Human judgment required."
                    Example: safety bypass detected, 3rd compaction reached
```

### Escalation Rules

1. **No skipping levels:** A P2 alert does not escalate to the Meta-Orchestrator
   unless it persists for 3+ scan cycles, at which point it becomes P1.
2. **Automatic promotion:** Alerts that persist without resolution are promoted
   one severity level per 3 scan cycles.
3. **No suppression:** Alerts cannot be suppressed or dismissed by the role
   they are escalated to. They can only be resolved (root cause addressed)
   or acknowledged (with justification logged to decision trail).
4. **Independence:** The Overseer's alert feed is append-only. No other role
   can modify or delete alerts. Resolution is a separate record that references
   the original alert.

### Monitoring Patterns

The Overseer implements the pattern detection playbook from
meta-orchestrator-mission.md, formalized as detection rules:

| Pattern | Detection Rule | Severity |
|---------|---------------|----------|
| Repeated permission prompts | Same `pattern_key` 3+ times across sessions | P2 → P1 |
| Repeated hook errors | Same hook error message in 2+ sessions | P2 |
| Sessions asking same question | Similar prompt patterns in 2+ capture-panes | P2 |
| Error messages teaching wrong behavior | Destructive flag in error message output | P1 |
| Workaround becoming default | Same workaround applied 3+ times without fix session | P1 |
| Session pivoting to docs/planning | Implementation session producing only `.md` files | P1 |
| Orchestrator implementing | Edit/Write tool calls in orchestrator capture-pane | P0 |

### Async Operation Model

The Overseer operates asynchronously relative to the Orchestrator's scan loop.
It does not block dispatch or execution. Its monitoring cadence is independent:

- **Observation interval:** Every scan cycle (same cadence as Orchestrator)
- **Deep scan:** Every 10th cycle (full re-read of all session states)
- **Read-only:** The Overseer only reads session state; it never modifies sessions
- **Output:** Alerts written to `~/.agm/logs/overseer-alerts.jsonl`

### Existing Implementation Mapping

| Overseer Function | Current Implementation |
|-------------------|----------------------|
| Pattern detection | Meta-orchestrator pattern detection playbook |
| Permission pattern analysis | orchestrator-mission.md § Permission Pattern Refine Loop |
| Stall detection | Orchestrator scan loop (session progress evaluation) |
| Escalation | Manual (meta-orchestrator → human) |
| Alert logging | Reasoning trace log (partially) |

### Relationship to Meta-Orchestrator

The Overseer and Meta-Orchestrator have distinct responsibilities:

| Concern | Overseer | Meta-Orchestrator |
|---------|----------|-------------------|
| Scope | Anomaly detection | System governance |
| Action | Alert (never act) | Decide and direct |
| Cadence | Every cycle | On alert or scheduled |
| Authority | None (watchdog) | System-level decisions |

The Overseer feeds the Meta-Orchestrator. The Meta-Orchestrator acts on
Overseer alerts. The Overseer cannot be overridden by the Meta-Orchestrator
(its alerts persist regardless of governance decisions).

## Alternatives Considered

1. **Orchestrator self-monitors (status quo):** Conflict of interest — the
   Orchestrator launched the sessions and has queue-clearing incentive.
   Independent monitoring catches what self-monitoring misses.
2. **Meta-Orchestrator handles all monitoring:** Overloads the governance role
   with operational monitoring. The Meta-Orchestrator should make decisions
   based on Overseer data, not collect the data itself.
3. **Passive logging only (no escalation):** Monitoring without escalation is
   audit theater. Alerts must route to someone who can act.

## Consequences

- Anomaly detection is decoupled from execution — the Overseer has no
  incentive to suppress problems
- The escalation ladder provides predictable response times and routing
- Alert persistence (append-only, no suppression) creates a complete
  audit trail of everything the system detected
- The Overseer adds monitoring overhead but reduces the cost of undetected
  failures (cascading failures, false completions, safety bypasses)
- Implementation may start as rules within the meta-orchestrator session,
  then migrate to astrocyte as detection rules become deterministic

## Cross-References

- [ADR-020: VROOM Architecture Overview](ADR-020-vroom-architecture-overview.md)
- [ADR-021: Verifier Role](ADR-021-verifier-role.md) — Overseer triggers spot verification
- [ADR-023: Orchestrator Role](ADR-023-orchestrator-role.md) — Overseer monitors sessions
- [ADR-025: Meta-Orchestrator Role](ADR-025-meta-orchestrator-role.md) — receives P1 alerts
- [Meta-Orchestrator Mission § Pattern Detection](../meta-orchestrator-mission.md)
- [Orchestrator Mission § Permission Pattern Refine Loop](../orchestrator-mission.md)
- [DEAR Protocol § Audit](../DEAR-PROTOCOL.md)
