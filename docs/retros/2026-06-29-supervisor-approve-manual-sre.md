# Retro: Manual SRE Intervention to Add agm send approve to Supervisor Skills

**Date:** 2026-06-29  
**Bead:** ce-20p9  
**Severity:** P0 — mesh-level deadlock risk

## What Happened

A supervisor permission audit found that `agm send approve` is never mentioned in any
of the three VROOM supervisor skill files, despite ADR-002 requiring mutual-unblock
between supervisors.

When the host tried to dogfood this by dispatching bead ce-20p9 to the VROOM supervisors
for them to self-fix, all three supervisors were found to be STALE/uninitialized:

- `vroom-meta-orchestrator`: `session_uninitialized` — Claude process not running; pane
  had unsent text "stop the loop" frozen in the input buffer
- `vroom-orchestrator`: detached (CDX harness), no permission prompt to approve
- `vroom-overseer`: detached (AGY harness), no permission prompt to approve

`agm supervisor status` confirmed all three STALE with heartbeat ages of 10k–63k seconds.

Because the supervisors were genuinely down, the host had to implement Fix 1 and Fix 2
directly as an SRE intervention.

## Root Cause

Two distinct failures:

1. **Missing `agm send approve` in skill files** — supervisors had no instruction to try
   `agm send approve` during a peer-stale response. A peer blocked on a permission prompt
   cannot receive `agm send msg`; without the approve step, one stuck supervisor can cause
   cascading STALE across the mesh.

2. **Supervisors were down** — the audit gap ironically prevented the supervisors from
   picking up their own fix ticket. Exact cause of shutdown not determined in this session
   (prior context lost across reboot/session boundary).

## What Was Fixed (this PR)

**Fix 1 (Critical):** Added `agm send approve <peer>` to Step 1 peer-check in all three
skill files. The call is idempotent — it exits cleanly if no permission prompt is present —
so adding it unconditionally on every stale heartbeat is safe.

**Fix 2 (Critical):** Overseer Step 6 supervisor-stuck handler updated to call
`agm send approve <supervisor-session>` instead of the previous description ("send urgent
status ping") which was both vague and wrong — a session in PERMISSION_PROMPT cannot
receive an `agm send msg` ping at all.

## What Was NOT Fixed (in ce-20p9, still open)

- Fix 3: `agm scan --cross-check` in Orchestrator Step 5
- Fix 4: Orchestrator Level 2 worker PERMISSION_PROMPT response
- Fix 5: Pre-approve `&` background operator in supervisor settings
- Fix 6: Dedicated permission-prompt watchdog launchd sidecar

These four items remain as open work items in ce-20p9.

## Recurrence Prevention

- Supervisors should now self-unblock on the next tick after a permission-prompt stall,
  because all three peer-check paths now call `agm send approve`.
- The Overseer's supervisor-stuck handler now emits a concrete unblock command rather than
  an ambiguous "ping."

## Open Questions

- Why were all three supervisors simultaneously down? The root cause of the shutdown was
  not investigated in this session. See VROOM reboot-recovery notes for the restart recipe.
- Should ce-20p9 Fixes 3–6 be dispatched to supervisors once they're back up, as a
  dogfood validation that the Fix 1 change actually works?
