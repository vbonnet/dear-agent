# Design Spike: VROOM Supervisor Leader-Election and Failover

**Status**: Spike (design only — implementation tracked separately as W0)
**Date**: 2026-06-20
**Bead**: ce-bew7
**Context**: Retro `2026-06-17-vroom-overnight-overseer-failover-dear-retro.md`
**Grounding**: [ADR-002](adr/ADR-002-vroom-execution-architecture.md) (the
three-supervisor mesh), [/CONTEXT.md](../CONTEXT.md) (role vocabulary).

## Problem

When the Meta-Orchestrator goes dark, the VROOM mesh stops permanently: no
peer attempts takeover, and the surviving supervisors zombie-loop — the
2026-06-17 incident saw the Orchestrator burn ~3 hours of tokens doing
nothing. The mesh has no concept of quorum, no degraded mode, and no way for a
Secondary to assume a dark peer's responsibilities. This spike designs the
protocol that closes those gaps (retro R.1–R.5).

The mesh is asymmetric by role. Each supervisor has a fixed **Secondary** (who
verifies and covers for it) and **Tertiary** (who unsticks it), per ADR-002:

| Target | Secondary (covers) | Tertiary (unsticks) |
|---|---|---|
| Meta-Orchestrator | Overseer | Orchestrator |
| Orchestrator | Meta-Orchestrator | Overseer |
| Overseer | Orchestrator | Meta-Orchestrator |

## 1. Zombie self-termination

A supervisor self-terminates after **N = 10 consecutive iterations** in which
*all* peers appear dark (configurable; default chosen so a ~2-minute loop
tolerates ~20 minutes of total mesh silence before giving up). The trigger is
the per-iteration peer-staleness signal already produced by the loop's
`CheckSkill`: each iteration where every peer is stale increments an
`allPeersDarkRun` counter; any fresh peer heartbeat resets it to zero. On
reaching the threshold the loop commits its decision-trail state and exits
cleanly with a sentinel error (`ErrAllPeersDark`) rather than continuing to
spin. Self-termination is the **emergency-mode (1/3)** behaviour: the survivor
is the last one standing, automated recovery has already failed, so it pages a
human and stops instead of consuming tokens indefinitely.

Distinction: *zombie self-termination* (all peers dark — give up) is separate
from *leader election* (one peer dark, quorum intact — take over). They share
the staleness signal but trigger opposite responses.

## 2. Leader-election protocol (claim → verify → assume → announce)

**Who triggers it.** Only a target's **Secondary** may contest it; the
**Tertiary** promotes itself *only if the Secondary is also dark*. This keeps
exactly one natural challenger per target and avoids a three-way scramble.

**Candidacy signal.** A peer is a candidate target once it has been stale for
**> 3 consecutive check intervals** (`PromoteThreshold`). Heartbeat freshness
is the candidacy field: the loop records each peer's `LastHeartbeat`; staleness
= `now − LastHeartbeat > 3 × interval`.

**Phases** (one phase advanced per loop iteration):

1. **Claim** — the eligible contender writes a `Claim{Target, By, Reason,
   Timestamp}` to a shared **ClaimStore** and opens a verify window.
2. **Verify** — the claim waits **2 check intervals** (`VerifyWindow`) before
   assuming. On expiry it re-reads the store; if a competing claim *wins*
   (earlier timestamp; ties broken by canonical role order), the contender
   **yields**. This window is the split-brain guard — two supervisors cannot
   both assume because they converge on the same deterministic winner.
3. **Assume** — the winner records `supervisor.failover.assumed` and begins
   discharging the target's responsibilities.
4. **Announce** — it notifies surviving peers (via an AGM message to their
   sessions) and appends the assumption to the decision trail.

The ClaimStore must be **file-backed** (`~/.agm/supervisors/<by>/`) so claims
survive across the separate AGM sessions each supervisor runs in.

## 3. Degraded-mode capability matrix

`MeshMode` derives from how many of the two peers the supervisor observes
alive (it always counts itself):

| Capability | 3/3 Normal | 2/3 Degraded | 1/3 Emergency |
|---|---|---|---|
| Add roadmap items | Meta-O only | Meta-O **or failover-authority holder** | ✗ |
| Spawn workers | ✓ | ✓ (rate ×0.5) | ✗ |
| Spawn rate limit | 1.0 | 0.5 | 0 |
| Elevated monitoring | ✗ | ✓ | ✓ |
| Self-terminate | ✗ | ✗ | ✓ |
| Notify human | ✗ | ✗ | ✓ |

Every mode transition is recorded to the decision trail with the in-force
capability set. Degraded mode does not stop work — it throttles spawning and
shortens the peer-check interval so a shrunken mesh neither over-commits nor
misses a second failure.

## 4. Roadmap delegation under failover

Single-roadmap-authority (ADR-002) still holds: normally only the
Meta-Orchestrator admits work. Under failover, the supervisor that has
**assumed** Meta-O authority writes through a **DelegatedRoadmap** seam that
gates every admission on a live `HoldsAuthority(meta-orchestrator)` check.
Items admitted this way are flagged `under_failover_authority` +
`subject_to_review` so the Meta-Orchestrator can audit them on return. The gate
is checked per-write, so the delegate **cannot** admit work the instant the
Meta-Orchestrator recovers and the assumption is relinquished.

## 5. Recovery to normal

Recovery is automatic and continuous. The election state machine keeps
stepping each assumed target; the moment the target's heartbeat is fresh again
(`isStale(target) == false`), the holder **relinquishes**: it deletes its claim
from the store, drops the assumed authority, records
`supervisor.failover.relinquished`, and the mesh returns to 3/3. No handshake
from the recovered peer is required — relinquishment is driven entirely by the
survivor observing renewed freshness, which avoids a recovered-but-silent peer
stalling the handback. Roadmap items added under failover remain flagged for
Meta-O review.

## 6. W0 implementation requirements

The implementation bead (W0) must deliver:

- **Loop wiring**: zombie counter + `ErrAllPeersDark` self-termination; the
  per-iteration staleness map fed to a `FailoverObserver`.
- **Election state machine**: per-target claim/verify/assume/announce with
  eligibility (Secondary always; Tertiary iff Secondary also dark), the
  deterministic `claimWins` tiebreak, and relinquish-on-recovery.
- **File-backed ClaimStore** under `~/.agm/supervisors/` for cross-session
  claims, plus an in-memory store for tests.
- **Capability matrix** (`MeshMode` + `CapabilitiesFor`) consulted before any
  spawn/roadmap action, with trail records on transition.
- **DelegatedRoadmap** gating roadmap writes on assumed authority, flagging
  failover-admitted items for review.
- **PeerNotifier / AGMRecovery**: `agm send wake-loop` to unstick a stale peer
  (mutual-unblock) and to announce assumption to survivors.
- **Tunable thresholds**: `ZombieThreshold` (10), `PromoteThreshold` (3),
  `VerifyWindow` (2× interval), recovery debounce (1) — all overridable.
- **Tests**: split-brain (two contenders, one winner), Tertiary deferral while
  Secondary alive, relinquish-on-recovery, and the all-peers-dark termination
  path.

## Alternatives rejected

- **External sentinel only.** The pre-incident `LoopMonitor` lived outside the
  mesh and could not take over; failover must be in-loop.
- **Symmetric any-peer election.** Without the fixed Secondary/Tertiary order a
  dark target draws two simultaneous claimants and split-brain risk rises; the
  asymmetric order makes the winner deterministic.
- **Quorum-less "keep trying forever".** That is exactly the 3-hour zombie loop
  the retro indicts; bounded self-termination is the fix.
