# agm/internal/ops — Requirements Specification (EARS)

<!-- Last audited at: NEEDS-AUDIT -->

**Version**: 1.0
**Last Updated**: 2026-06-07
**Status**: Baseline (derived from tests + code, not design-forward)
**Scope**: Shared business-logic layer for AGM CLI, MCP, and Skills surfaces

---

## Overview

`agm/internal/ops` is the shared implementation layer behind all three AGM API
surfaces (CLI, MCP server, Skills plugin). Every surface constructs an
`OpContext` and delegates here; no surface may call storage or tmux directly.

---

## EARS Requirements

### Session Listing

**OPS-01** When `ListSessions` is called with no filter, the system shall return
up to 100 sessions sorted by last-updated descending, each annotated with a
live tmux status ("active" or "stopped").

**OPS-02** When `ListSessions` is called with `limit > 1000`, the system shall
return an `OpError` with code `invalid_input` and reject the request without
querying storage.

**OPS-03** When `ListSessions` is called with `status = "active"`, the system
shall exclude sessions that have no live tmux pane from the result set.

**OPS-04** When `ListSessions` is called with `status = "archived"`, the system
shall return only sessions whose `Lifecycle` field equals `archived`.

**OPS-05** When `ListSessions` is called with an unrecognised `status` value,
the system shall return an `OpError` with code `invalid_input`.

**OPS-06** When a tmux session exists that has no corresponding AGM manifest,
the system shall surface it as an orphan entry in the list response.

**OPS-07** When a session's `LastKnownCost` is zero, the system shall compute
an estimated cost from token counts using the Opus pricing schedule
(input: $15/MTok, output: $75/MTok) before returning the summary.

### Session Resolution

**OPS-08** When `GetSession` is called with an exact session ID, the system
shall return the matching session detail without performing a name scan.

**OPS-09** When `GetSession` is called with a string that does not exactly
match any session ID, the system shall perform a full-table name scan
comparing `manifest.Name` and `manifest.Tmux.SessionName`.

**OPS-10** When a session has no tmux interface, the system shall report its
status as "unknown" rather than "active" or "stopped".

**OPS-11** When a session's `Lifecycle` is `archived`, the system shall report
its status as "archived" regardless of tmux state.

### Session Kill

**OPS-12** When `KillSession` is called on a session that has a live tmux pane
and `ConfirmedStuck` is false, the system shall return `ErrActiveSessionKill`
and not execute the kill.

**OPS-13** When `KillSession` is called on a session that was active within the
recency threshold and `Force` is false, the system shall return
`ErrKillProtected` and not execute the kill.

**OPS-14** When `KillSession` is called with `DryRun = true`, the system shall
return the would-be result without mutating tmux or storage.

**OPS-15** When `KillSession` is called on an already-archived session, the
system shall return an error indicating the session is not killable.

### Session Archive

**OPS-16** When `ArchiveSession` is called on a session that has a live tmux
pane and `Force` is false, the system shall block the archive and return an
error identifying the active pane.

**OPS-17** When `ArchiveSession` is called on a session whose working-directory
verification finds critical issues and `Force` is false, the system shall block
the archive and surface the verification failures.

**OPS-18** When `ArchiveSession` is called on a session whose name matches a
supervisor pattern (e.g. "orchestrator", "meta-orchestrator", "overseer") and
`Force` is false, the system shall block the archive.

**OPS-19** When `ArchiveSession` succeeds, the system shall in sequence:
set `Lifecycle = archived`, update storage, record a trust event, deregister
from the monitor, kill MCP processes, kill the tmux process group, and run
worktree/sandbox cleanup.

**OPS-20** When `ArchiveSession` is called on a session that is already
archived, the system shall return an error without re-running any cleanup step.

### Garbage Collection

**OPS-21** When `GC` is called, the system shall perform a pre-flight health
check by listing all sessions; if storage is unreachable it shall abort with a
503-equivalent error before touching any session.

**OPS-22** When `GC` evaluates a session, the system shall skip it with reason
`GCSkipActiveTmux` if the session has a live tmux pane.

**OPS-23** When `GC` evaluates a session, the system shall skip it with reason
`GCSkipActiveState` if the session manifest state is any of: WORKING,
PERMISSION_PROMPT, COMPACTING, WAITING_AGENT, LOOPING, BACKGROUND_TASKS,
USER_PROMPT, READY.

**OPS-24** When `GC` evaluates a session, the system shall skip it with reason
`GCSkipProtectedRole` if the session name contains any protected role substring
(default: "orchestrator", "meta-orchestrator", "overseer"; case-insensitive).

**OPS-25** When `GC` is called with `OlderThan` set, the system shall skip any
session whose `max(UpdatedAt, StateUpdatedAt)` is more recent than the
threshold, with reason `GCSkipTooRecent`.

**OPS-26** When `GC` is called with `DryRun = true`, the system shall record
GC intent in the log entries but not mutate storage or call `ArchiveSession`.

**OPS-27** When `GC` archives a session, the system shall write an entry to
`gc.jsonl` via the gclog subsystem for every action taken (skip or archive).

### Message Delivery

**OPS-28** When `SendMessage` is called with an empty recipient, the system
shall return a validation error without attempting delivery.

**OPS-29** When `SendMessage` is called with an empty message body, the system
shall return a validation error without attempting delivery.

**OPS-30** When `SendMessage` is called targeting an archived session, the
system shall return an error before attempting delivery.

**OPS-31** When a Manager backend is present, `SendMessage` shall deliver via
`manager.Backend.SendMessage` using the tmux session name as the session
identifier.

### Stall Detection

**OPS-32** When a session has been in `PERMISSION_PROMPT` state for longer than
`PermissionTimeout` (default 5 minutes), the stall detector shall classify it
as a critical stall.

**OPS-33** When a session named or tagged as "worker" has been in `WORKING`
state for longer than `NoCommitTimeout` (default 15 minutes) with zero git
commits since `StateUpdatedAt`, the stall detector shall classify it as a
warning stall.

**OPS-34** When the tmux pane output of a session contains any error pattern
repeated `ErrorRepeatThreshold` or more times, the stall detector shall
classify it as a warning stall.

**OPS-35** When a stall is detected, the system shall record it via
`recordErrorMemory` and publish a stall event to the `eventbus.Broadcaster`
if one is wired.

**OPS-36** While a session's state is OFFLINE, READY, or DONE, the stall
detector shall skip error-loop detection for that session.

### Field Mask Projection

**OPS-37** When `ApplyFieldMask` is called with a non-empty field list, the
system shall return a JSON object containing only the requested top-level keys.

**OPS-38** When `ApplyFieldMask` is called on a value that is not a JSON
object, the system shall return the value unchanged.

---

## Key Invariants

- **No surface bypasses ops.** CLI, MCP, and Skills all go through `OpContext`
  functions; direct storage or tmux calls from a surface are a violation.
- **GC skip priority is deterministic.** The `gcSkipReason` function applies
  checks in a fixed order: already-archived → reaping → protected-role →
  active-tmux → active-state → too-recent. The first matching check wins.
- **State field removed.** The `State` field was removed from `SessionSummary`
  because it produced false positives causing cascading bad decisions. Do not
  re-add it without an ADR.
