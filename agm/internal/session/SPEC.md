# agm/internal/session — Requirements Specification (EARS)

<!-- Last audited at: 2026-07-21 -->

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

**SESS-06** When aggregate workspace status is encoded as JSON, the system shall expose stable lower-snake-case keys for the workspace, summary counts, and detailed session fields rather than Go field names.

**SESS-07** When AGM detects context for a Pi manifest, the system shall resolve the exact persisted native transcript identity and reject a transcript-path mismatch.

**SESS-08** When AGM converts Pi native usage into context percentage, the system shall preserve case-sensitive route identity and the exact route-specific Pi 0.81 model-catalog window for known direct or nested OpenRouter provider-qualified models and a conservative documented fallback for unknown or multiply qualified direct-provider models.

**SESS-09** When a Pi transcript identifies a configured model declaration or a model override for a Pi 0.81 built-in provider, the system shall read only a bounded non-symlink regular `models.json` that is not writable by group or other users, shall resolve the case-sensitive provider separately from the complete opaque model ID with Pi 0.81 declaration and override precedence independently of AGM's static model-window table, shall distinguish an omitted custom-model window from an explicit null, shall accept only positive integral explicit context windows no larger than 16,777,216 tokens, and shall fall back without evaluating credential or header values when the catalog or match is absent, ambiguous, malformed, unsafe, invalid, or oversized.

## Key Invariants

- **Capability, not contract widening.** `HarnessLivenessChecker` is a
  separate optional interface; `TmuxInterface` itself is unchanged so mocks
  and adapters outside this package are not broken by liveness support.
- **Fail-safe liveness.** A failed or unavailable liveness scan proves
  nothing: callers must fall back to tmux session existence, never treat an
  unverifiable session as dead.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/pi_custom_context.feature`
