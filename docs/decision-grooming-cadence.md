# Decision: Backlog Grooming Cadence and Ownership

**Status:** Accepted
**Date:** 2026-06-21
**Bead:** ce-h59v
**Companions:** [[ce-ynyb]] spike pattern adoption

---

## Context

The mesh carries **195 open P0+P1 beads** (30 P0, 165 P1). P1 inflation has made
priority meaningless — *if everything is P1, nothing is P1.* Agents burn tokens
reading stale, duplicate, and zombie beads (merged PR, still OPEN), and cannot
distinguish a genuine blocker from aspirational work. There is no owner for
backlog health and no regular cadence to correct drift. This decision assigns
both.

---

## Decision

### 1. Ownership

**The meta-orchestrator owns grooming** as the CTO-analog that owns the roadmap.
Grooming is a **supervisor responsibility — never delegated to workers.**
For any **P0 reprioritization** (creating, closing, or downgrading a P0),
**orch and overseer consensus is required**; the meta-orchestrator may not
unilaterally change P0 status. All other grooming actions (P1/P2 triage, dedup,
zombie close) are the meta-orchestrator's call.

### 2. Cadence

Grooming runs on a **periodic clock OR a backlog trigger, whichever fires first:**

- **Periodic:** every **480 ticks (~24h at 3min/tick)** — one session per day.
- **Trigger:** also run immediately if **P1 count > 150** OR **P0 count > 25**.

The trigger is the safety valve; the periodic clock is the floor. A trigger-fired
session resets the periodic timer.

### 3. Process

A grooming session works a fixed checklist:

1. **Close stale beads** — OPEN beads untouched > 7 days with no hold reason.
2. **Dedup** — merge semantic duplicates into a canonical bead; close duplicates
   with a `blocks:`/link reference, never a silent delete.
3. **Downgrade inflated P1s** — apply the *"would we drop this in a crunch?"* test.
   If yes → P2. P1s unworked > 7 days without a hold reason are demoted.
4. **Audit P0s** — confirm each P0 is still a genuine blocker (consensus gate per §1).
5. **Catch zombies** — beads with a MERGED PR but OPEN status → close with a DoD note.
6. **Archive deferred work** — work parked indefinitely moves out of the active queue.
7. **Classify intake** — assign priority to unclassified beads since the last groom.

**Inflation prevention (standing rules between sessions):** new beads default to
**P2** unless a one-line justification argues P1+ at creation time.

### 4. Timing gate

Grooming **runs only during low-swap / gate-OPEN periods.** Mutating priorities
and closing beads during a high-contention window competes with active workers for
the bead DB and risks reprioritizing work mid-flight. If the gate is CLOSED when a
session is due, it defers to the next OPEN window — the cadence clock holds, it does
not skip.

### 5. Tooling

Grooming is executed with the **`bd` CLI** against
`~/beads/context-engine/.beads`, run by the meta-orchestrator supervisor (not a
dedicated grooming bead — the cadence is a standing supervisor duty, not a unit of
delivered work). Each completed session is recorded as a **trail entry**
(`supervisor.meta.grooming_complete`) capturing counts before/after, beads closed,
and any P0 consensus decisions. Cross-supervisor outcomes propagate via **handoff
advisories** so orch and overseer see the post-groom state.

### 6. Success metric

Grooming is working when the backlog holds at **P0 < 20** and **P1 < 100** on a
rolling basis (down from 30/165). Secondary signals: zero zombie beads at session
end, and every P0 traceable to a named blocker. A session that cannot bring P1
under 100 flags the overflow in its trail entry for the next session.

---

## Consequences

**Positive:** priority regains signal; agents stop paying token rent on dead beads;
backlog health has a named owner and an auditable cadence; the consensus gate keeps
any single supervisor from quietly reshaping P0.

**Negative / trade-offs:** daily grooming is recurring supervisor overhead, and the
P0 consensus gate adds latency to reprioritization (intended — P0 churn should be
deliberate). The 7-day staleness and *"drop it in a crunch?"* tests are
judgment-based; the trail entry makes each call reviewable after the fact.

**Awareness:** this cadence propagates through the meta-orchestrator's standing
duties and the supervisor playbook so it survives handoffs.
