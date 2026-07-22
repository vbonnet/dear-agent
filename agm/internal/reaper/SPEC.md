# agm/internal/reaper — Requirements Specification (EARS)

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
- Feature: `agm/test/bdd/features/harness_parity.feature`

<!-- Last audited at: 2026-07-22 -->

**Version**: 1.1
**Last Updated**: 2026-07-22
**Status**: Living
**Scope**: Two-phase stop-then-archive lifecycle for AGM-managed harness sessions

---

## Overview

The reaper executes a deterministic, crash-recoverable two-phase sequence to
terminate an AGM harness session: Phase 0 (safety and shared archive preflight)
→ Phase 1 (stop) → Phase 2 (`ops.ArchiveSession`). The invariant is that
lifecycle storage is never updated to `archived` while a tmux pane is still
alive.

---

## EARS Requirements

### Construction

**REA-01** When `New(sessionName, sessionsDir, logger)` is called, the system shall resolve the tmux socket path from the AGM global socket at construction time and store it on the struct.

**REA-02** When `sessionsDir` is empty, the system shall default to `~/.claude/sessions` as the sessions directory.

**REA-03** When `sessionsDir` is non-empty, the system shall use it as-is without modification.

**REA-04** When two `Reaper` instances are created, the system shall share the same `SocketPath` value and hold independent `SessionName` and `SessionsDir` values.

### Safety Check (Phase 0)

**REA-05** When `Run()` is called and `safety.Check()` detects a human in the session, the system shall abort before taking any other action and return an error.

**REA-18** When archive preflight finds a protected supervisor, critical completion-verification failure, or pending delegation and force is false, the system shall abort before writing the reaping tombstone or touching tmux.

### Crash-Recovery Tombstone

**REA-06** When Phase 1 begins, the system shall call `markReaping()` before any tmux interaction.

**REA-07** When `markReaping()` finds the session is already archived, the system shall return without error.

### Stop Sequence (Phase 1)

**REA-08** When attempting to stop the session, the system shall wait up to 90 seconds for the agent prompt to appear.

**REA-09** When the agent prompt is detected, the system shall select the
harness-native graceful-exit command from the persisted session harness and
send it via `tmux.SendMultiLinePromptSafe`: Pi (`pi-cli` and legacy `pi`) uses
`/quit`; every other supported or unknown harness retains `/exit`.

**REA-10** When the native graceful-exit command has been sent, the system shall wait up to 60 seconds for the pane to close.

**REA-11** When the `ReaperTimeout` budget is exhausted before the native graceful-exit command can be sent, the system shall mark the session as a zombie and escalate to force-kill.

**REA-12** When escalating to force-kill, the system shall send SIGTERM before SIGKILL.

**REA-13** When kill attempts complete and the pane is still alive, the system shall return an error rather than proceeding to Phase 2.

### Archive Sequence (Phase 2)

**REA-14** When pane death is confirmed, the system shall proceed to `archiveSession()`.

**REA-15** When resource cleanup is running and MCP process cleanup fails, the system shall continue the archive sequence.

**REA-19** When pane death is confirmed, the system shall complete archival through `ops.ArchiveSession`, preserving its durable transition, outcome stamping, external archive outcomes, and cleanup contract without a reaper-owned final lifecycle mutation.

**REA-20** When `agm session archive --async` supplies force, keep-sandbox, or outcome options, the system shall propagate those options across the detached `agm-reaper` process boundary and apply them to shared preflight and final archival.

**REA-21** When one reaper run performs preflight, writes the reaping tombstone, and finalizes archival, the system shall reuse one migrated lifecycle-storage connection across those phases.

### Timing Constants (regression-pinned)

**REA-16** When reaper timing constants are evaluated, the system shall preserve the pinned timeout values covered by `TestConstantValues`.

**REA-17** When a reaper inherits `AGM_DB_PATH` from an isolated AGM test environment, the system shall use that persistent SQLite lifecycle store for both its reaping tombstone and final archive, and shall not run production worktree cleanup against the isolated store.

---

## Key Invariants

- **Pane-before-archive ordering.** Phase 2 (`archiveSession`) is only
  reachable after `tmux.IsPaneActive()` returns false. A non-nil error from
  that check aborts `Run()` entirely.
- **Crash recovery via `reaping` tombstone.** `markReaping()` writes the Dolt
  tombstone before any tmux interaction. A GC startup scan detecting
  `lifecycle=reaping` can finish the interrupted reap.
- **Shared finalizer.** `markReaping()` is the reaper's only direct lifecycle
  write. Phase 2 calls `ops.ArchiveSession`; copied archive/update/cleanup code
  in this package is forbidden.
- **No retry on permission errors.** If safety.Check blocks or storage is
  unreachable, `Run()` returns immediately. Callers must not retry blindly;
  they must report and escalate.
- **Native shutdown ownership.** `GracefulExitCommand` is the single owner of
  harness shutdown-command selection. Detached reapers must derive the harness
  from durable session state rather than ambient caller configuration.
