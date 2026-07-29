# agm/internal/session — Requirements Specification (EARS)

<!-- Last audited at: 2026-07-22 -->

## Purpose

`agm/internal/session` owns session lifecycle state: manifest-derived status
computation, tmux abstraction (`TmuxInterface` + real/mock implementations),
session state detection, and completion verification. It is the boundary
through which the ops layer observes tmux without importing the tmux package
directly. `RealTmux` is the one production local runtime adapter and exposes
focused strict-existence, liveness, startup-readiness, atomic-input, and
exact-pane capabilities.

## EARS Requirements

**SESS-01** When session status is computed for an archived manifest, the system shall report `archived` regardless of tmux state.

**SESS-02** When session status is computed and the tmux session does not exist (or tmux is unavailable), the system shall report `stopped`.

**SESS-03** When batch status is computed for lifecycle decisions, the system shall key results by session ID (not by Name) so shadow manifests sharing a Name cannot hide a live session.

**SESS-04** When a tmux runtime provides the `HarnessLivenessChecker` capability, the system shall expose the harness-process verdict (session existence, harness liveness, zombie-writer flag, and pane-tree evidence) from the full pane descendant-tree scan in `agm/internal/tmux`, so callers can distinguish a live session from a zombie pane whose harness has died (ce-axsr).

**SESS-05** When a tmux runtime does not implement `HarnessLivenessChecker`, the system shall let callers discover the capability by type assertion so existing `TmuxInterface` implementations keep compiling and fall back to session-existence semantics.

**SESS-06** When aggregate workspace status is encoded as JSON, the system shall expose stable lower-snake-case keys for the workspace, summary counts, and detailed session fields rather than Go field names.

**SESS-07** When AGM detects context for a Pi manifest, the system shall resolve the exact persisted native transcript identity and reject a transcript-path mismatch.

**SESS-08** When AGM converts Pi native usage into context percentage, the system shall preserve case-sensitive route identity and the exact route-specific Pi 0.81 model-catalog window for known direct or nested OpenRouter provider-qualified models and a conservative documented fallback for unknown or multiply qualified direct-provider models.

**SESS-09** When a Pi transcript identifies a configured model declaration or an override matching its exact provider-qualified route, the system shall read only a bounded non-symlink regular `models.json` that is not writable by group or other users, shall resolve the case-sensitive provider separately from the complete opaque model ID with Pi declaration and override precedence independently of AGM's static model-window and provider tables, shall distinguish an omitted custom-model window from an explicit null, shall accept positive integral explicit context windows no larger than 16,777,216 tokens regardless of equivalent JSON integer, decimal, or exponent spelling, and shall fall back without evaluating credential or header values when the catalog or match is absent, malformed, unsafe, fractional, invalid, or oversized or when an unqualified model ID matches more than one provider regardless of equal values or validity.

**SESS-10** When AGM calculates context usage for a Pi session, the system shall resolve custom model metadata from the coding-agent directory persisted with that session, including an explicitly present empty value for Pi's native default, rather than from the status caller's current `PI_CODING_AGENT_DIR` environment; only metadata that genuinely predates the presence marker may use the caller environment as a compatibility fallback.

**SESS-11** When legacy Claude resume must create a replacement tmux session, the system shall stage the private launch handoff before allocating tmux; if command delivery then fails, the system shall cancel the handoff, remove only the session created by that attempt, and report any cleanup failure with the primary delivery error.

**SESS-12** When the AGM production composition root initializes local runtime behavior, the system shall construct one `RealTmux` and inject it directly through `TmuxInterface` without a backend adapter round trip.

**SESS-13** When `RealTmux` compiles, the system shall directly satisfy checked kill, strict existence, harness liveness, batch liveness, harness readiness, input readiness, atomic input delivery, and verified exact-pane delivery capabilities.

**SESS-14** When a future non-tmux runtime obtains a production caller, the system shall define the smallest capability seam required by that caller rather than restore a general lifecycle, messaging, and state facade.

## Key Invariants

- **Capability, not contract widening.** Focused capability interfaces keep
  `TmuxInterface` testable without granting every caller every mechanism.
- **One production adapter.** `RealTmux` adapts the tmux process exactly once;
  forwarding wrappers and parallel runtime registries are not architecture.
- **Fail-safe liveness.** A failed or unavailable liveness scan proves
  nothing: callers must fall back to tmux session existence, never treat an
  unverifiable session as dead.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/pi_custom_context.feature`
- Package regression: `agm/internal/session/resume_test.go`
