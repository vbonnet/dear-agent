# agm/internal/reaper — Requirements Specification (EARS)

<!-- Last audited at: NEEDS-AUDIT -->

**Version**: 1.0
**Last Updated**: 2026-06-07
**Status**: Baseline (derived from tests + code, not design-forward)
**Scope**: Two-phase stop-then-archive lifecycle for AGM-managed Claude sessions

---

## Overview

The reaper executes a deterministic, crash-recoverable two-phase sequence to
terminate a Claude Code session: Phase 0 (safety check) → Phase 1 (stop)
→ Phase 2 (archive). The invariant is that Dolt is never updated to `archived`
while a tmux pane is still alive.

---

## EARS Requirements

### Construction

**REA-01** When `New(sessionName, sessionsDir, logger)` is called, the system
shall resolve the tmux socket path from the AGM global socket at construction
time and store it on the struct.

**REA-02** When `sessionsDir` is empty, the system shall default to
`~/.claude/sessions` as the sessions directory.

**REA-03** When `sessionsDir` is non-empty, the system shall use it as-is
without modification.

**REA-04** When two `Reaper` instances are created, they shall share the same
`SocketPath` value (the global tmux socket is a singleton) and hold independent
`SessionName` and `SessionsDir` values.

### Safety Check (Phase 0)

**REA-05** When `Run()` is called and `safety.Check()` detects a human is
present in the session, the system shall abort before taking any other action
and return an error.

### Crash-Recovery Tombstone

**REA-06** When Phase 1 begins, the system shall call `markReaping()` before
any tmux interaction, writing `lifecycle=reaping` to Dolt, so that a crash
mid-sequence leaves a recoverable tombstone.

**REA-07** When `markReaping()` finds the session is already archived, it shall
return without error (idempotent).

### Stop Sequence (Phase 1)

**REA-08** When attempting to stop the session, the system shall wait up to 90
seconds for the Claude prompt to appear, with a 60-second fallback sleep if
detection fails.

**REA-09** When the Claude prompt is detected, the system shall send `/exit` via
`tmux.SendMultiLinePromptSafe` to request a graceful shutdown.

**REA-10** When `/exit` has been sent, the system shall wait up to 60 seconds
for the pane to close (allowing SessionEnd hooks to run).

**REA-11** When the `ReaperTimeout` budget (10 minutes) is exhausted before
`/exit` can be sent, the system shall mark the session as a zombie and escalate
to force-kill.

**REA-12** When escalating to force-kill, the system shall: send SIGTERM, wait
10 seconds (grace period), then send SIGKILL, then wait 5 seconds for the
process to exit.

**REA-13** After all kill attempts, the system shall verify pane death via
`tmux.IsPaneActive()`; if the pane is still alive, the system shall return an
error rather than proceeding to Phase 2.

### Archive Sequence (Phase 2)

**REA-14** When pane death is confirmed, the system shall proceed to
`archiveSession()`, which shall, in order: kill any lingering MCP processes
(best-effort), set `lifecycle=archived` in Dolt, run resource cleanup
(worktrees, branches, temp dirs), clean the pending directory
(`~/.agm/pending/<session>`), and archive the legacy session directory.

**REA-15** While resource cleanup is running, MCP process kill failures shall
not abort the archive sequence (best-effort semantics).

### Timing Constants (regression-pinned)

**REA-16** The following constants shall hold their exact pinned values:
`PromptDetectionTimeout = 90s`, `PaneCloseTimeout = 60s`,
`FallbackWaitTime = 60s`, `SIGTERMGracePeriod = 10s`,
`PostKillPaneTimeout = 5s`, `ReaperTimeout = 10min`.
Any change to a constant value requires a test update in `TestConstantValues`.

---

## Key Invariants

- **Pane-before-archive ordering.** Phase 2 (`archiveSession`) is only
  reachable after `tmux.IsPaneActive()` returns false. A non-nil error from
  that check aborts `Run()` entirely.
- **Crash recovery via `reaping` tombstone.** `markReaping()` writes the Dolt
  tombstone before any tmux interaction. A GC startup scan detecting
  `lifecycle=reaping` can finish the interrupted reap.
- **No retry on permission errors.** If safety.Check blocks or storage is
  unreachable, `Run()` returns immediately. Callers must not retry blindly;
  they must report and escalate.
