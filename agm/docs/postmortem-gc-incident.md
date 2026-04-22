# Post-Mortem: Session GC Archiving Active Supervisors

**Date**: 2026-04-10
**Severity**: P1 — Active supervisor sessions destroyed mid-operation
**Status**: Resolved (GC disabled; permanent fix pending)
**Author**: SRE retro session

---

## Summary

The session garbage collector (`session_gc.go`) archived active supervisor sandboxes
(meta-orchestrator, orchestrator) while they were running, causing cascading session
failures. An emergency fix (commit `65ec043f`) disabled GC entirely to stabilize the
system. A preceding memory pressure crash from 28 concurrent sessions and 233 stale
sandbox directories (14GB) compounded the incident.

---

## Timeline

| Time (approx.) | Event |
|----------------|-------|
| T-2h | 233 stale sandbox directories accumulate, consuming ~14GB disk |
| T-1h | 28 concurrent sessions cause memory pressure; system crashes |
| T-0 | Session GC triggers, archives sandboxes older than age threshold |
| T-0 | GC archives meta-orchestrator and orchestrator sandboxes (active) |
| T+5m | Supervisor sessions degrade — overlay filesystems invalidated |
| T+10m | Cascading failures across worker sessions managed by supervisors |
| T+15m | Manual investigation identifies GC as cause |
| T+20m | Emergency fix: commit `65ec043f` disables GC entirely |
| T+25m | System stabilized; supervisor sessions restarted manually |

---

## Root Cause

`session_gc.go` used a single criterion for archival: session age exceeding a
configurable threshold. It had **no awareness of session state**:

1. **No active-session check** — GC never queried `agm session list` to determine
   which sessions were currently running.
2. **No supervisor protection** — Orchestrator, meta-orchestrator, and overseer
   sessions were treated identically to ephemeral worker sessions.
3. **No heartbeat mechanism** — No signal existed for GC to distinguish a live
   sandbox from an abandoned one.

The GC operated on filesystem metadata (directory mtime) alone, which is insufficient
for determining liveness in a system where long-running supervisor sessions are
expected to persist indefinitely.

---

## Detection Gap

- **No pre-GC health check**: GC did not verify session liveness before archival.
- **No monitoring on sandbox deletion**: No alerts fired when sandbox directories
  were removed. The only signal was session degradation observed by users.
- **No heartbeat protocol**: Active sessions did not write periodic heartbeat files
  that GC could check, nor did GC query the session manager.
- **Stale accumulation went unnoticed**: 233 directories / 14GB built up with no
  alerting threshold on sandbox count or disk usage.

---

## Contributing Factors

1. **No max-session limit**: The system allowed 28+ concurrent sessions without
   backpressure, leading to the memory crash that preceded the GC incident.
2. **Reactive cleanup model**: GC was the only mechanism for managing sandbox
   lifecycle — no proactive limits prevented accumulation.
3. **Single threshold design**: Age-based archival is appropriate for abandoned
   sessions but dangerous without an active-session exclusion list.

---

## Prevention: Required Changes

### P0 — Must have before re-enabling GC

1. **Active session exclusion**: GC must call `agm session list` and skip any
   session with status `active`, `running`, or `paused`.
2. **Supervisor exclusion list**: Sessions with roles `orchestrator`,
   `meta-orchestrator`, or `overseer` must be permanently excluded from GC,
   regardless of age.
3. **Pre-GC health check**: Before any archival pass, GC must verify it can
   reach the session manager and get a valid session list. If the check fails,
   GC must abort the pass entirely.

### P1 — Should have soon

4. **Max session limit**: Enforce a configurable cap on concurrent sessions
   (e.g., 20) to prevent accumulation rather than relying on reactive cleanup.
5. **Disk usage alerting**: Alert when sandbox directory count exceeds threshold
   or total disk usage crosses a watermark.
6. **Heartbeat files**: Active sessions write a periodic heartbeat file; GC
   treats any session with a recent heartbeat as protected.

---

## Action Items

| # | Action | Priority | Owner |
|---|--------|----------|-------|
| 1 | Add `agm session list` query to GC before archival | P0 | agm team |
| 2 | Add supervisor role exclusion list to GC config | P0 | agm team |
| 3 | Add pre-GC health check (abort if session manager unreachable) | P0 | agm team |
| 4 | Re-enable GC with protections (revert `65ec043f` + new logic) | P0 | agm team |
| 5 | Implement max concurrent session limit | P1 | agm team |
| 6 | Add sandbox count / disk usage monitoring + alerts | P1 | agm team |
| 7 | Implement heartbeat file protocol for active sessions | P2 | agm team |
| 8 | Add integration test: GC must not archive active sessions | P1 | agm team |

---

## Lessons Learned

1. **Garbage collection without liveness checks is a footgun.** Any automated
   cleanup that can destroy active resources must verify liveness first.
2. **Supervisor sessions are not ephemeral.** The system must distinguish between
   short-lived workers and long-lived supervisors at the GC level.
3. **Prevention > reaction.** Max-session limits would have prevented the 233-session
   accumulation that made aggressive GC seem necessary.
4. **Silent deletion is dangerous.** Any automated deletion should log loudly and
   ideally alert before acting, especially for supervisor-class resources.
