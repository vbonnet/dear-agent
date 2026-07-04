# agm/internal/session — Requirements Specification (EARS)

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/session` owns session lifecycle state: manifest-derived status
computation, tmux abstraction (`TmuxInterface` + real/mock implementations),
session state detection, and completion verification. It is the boundary
through which the ops layer observes tmux without importing the tmux package
directly, including the optional harness-process liveness capability
(ce-axsr).

## EARS Requirements

**SESS-01** When session status is computed for an archived manifest, the system shall report `archived` regardless of tmux state.

**SESS-02** When session status is computed and the tmux session does not exist (or tmux is unavailable), the system shall report `stopped`.

**SESS-03** When batch status is computed for lifecycle decisions, the system shall key results by session ID (not by Name) so shadow manifests sharing a Name cannot hide a live session.

**SESS-04** When a tmux backend provides the `HarnessLivenessChecker` capability, the system shall expose the harness-process verdict (session existence, harness liveness, zombie-writer flag, and pane-tree evidence) from the full pane descendant-tree scan in `agm/internal/tmux`, so callers can distinguish a live session from a zombie pane whose harness has died (ce-axsr).

**SESS-05** When a tmux backend does not implement `HarnessLivenessChecker`, the system shall let callers discover the capability by type assertion so existing `TmuxInterface` implementations keep compiling and fall back to session-existence semantics.

## Key Invariants

- **Capability, not contract widening.** `HarnessLivenessChecker` is a
  separate optional interface; `TmuxInterface` itself is unchanged so mocks
  and adapters outside this package are not broken by liveness support.
- **Fail-safe liveness.** A failed or unavailable liveness scan proves
  nothing: callers must fall back to tmux session existence, never treat an
  unverifiable session as dead.
