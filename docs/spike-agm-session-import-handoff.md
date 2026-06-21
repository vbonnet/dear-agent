# Spike: `agm session import` handoff protocol

**Bead:** ce-akkp (spike) → feeds ce-dbgc (implementation)
**Status:** Investigation complete. Docs only — no code changed.
**Date:** 2026-06-20

## Goal

ce-dbgc proposes `agm session import <uuid> <model-family>` so Dispatch can hand
off an **already-running** Claude Code conversation to a supervisor for
monitoring without spawning a fresh session. This spike answers three open
questions about the handoff and what (if anything) already exists.

## TL;DR

- An `agm session import <uuid>` command **already exists** (`agm/cmd/agm/import.go`),
  backed by `importer.ImportOrphanedSession`. It writes the UUID→manifest binding
  that `associate` would otherwise write.
- **`/agm:agm-assoc` is NOT required for monitoring** when import already records
  the correct live tmux session name. agm-assoc's distinct value is (a) detecting
  the live tmux pane name from inside the pane and (b) emitting the ready-file.
  Monitoring (capture-pane) needs neither.
- The reusable core for ce-dbgc is **`importer.RegisterSession`** (idempotent,
  non-interactive, already hook-callable) — not `ImportOrphanedSession` (errors
  on duplicate, prompts for a name).

## How monitoring actually works

Supervision reads a pane, it does not "attach" to a process. The monitor calls
`tmux capture-pane -p -t <sessionName>` (`agm/internal/monitor/tmux/capture.go`).
The only binding monitoring needs is **manifest.Tmux.SessionName → a live tmux
session of that name**. The manifest also carries `Claude.UUID`, indexed in Dolt
on `agm_sessions.claude_uuid` (`Adapter.GetSessionByUUID`, O(1)).

So a session is monitorable the moment a manifest exists whose `Tmux.SessionName`
matches a running pane. UUID is needed for resume/re-detection, not for
capture-pane.

## Q1 — Does `/agm:agm-assoc` need to run in the tmux pane after import?

**No, not for monitoring** — provided import records the tmux session name that
the conversation is actually running in.

What `/agm:agm-assoc` does (`agm/agm-plugin/commands/agm-assoc.md` →
`agm session associate`, `agm/cmd/agm/associate.go`):

1. Resolves the session name from `tmux display-message -p '#S'` (it must run
   **inside** the live pane to learn the real name).
2. Detects the live Claude UUID from `~/.claude/history.jsonl` and writes it to
   the manifest (creating the manifest with `--create` if absent).
3. Creates the ready-file `$AGM_STATE_DIR/ready-{session}` via
   `readiness.CreateReadyFile`.

Overlap with import: step 2 (UUID→manifest binding) is exactly what
`import` already does. So re-running agm-assoc only adds value when:

- **import guessed the tmux name wrong.** import calls
  `tmux.SanitizeSessionName(sessionName)` on a name the operator supplies; it
  never verifies a live pane of that name exists. agm-assoc, run inside the pane,
  reads the *real* `#S`. If the handoff target's pane name ≠ the imported name,
  monitoring's capture-pane silently fails until corrected.
- **a waiter depends on the ready-file.** Only `agm session new`'s init path
  consumes it (`new_postcreate.go`, `new_associate.go`, `readiness.WaitForReady`).
  A monitor-only handoff has no such waiter, so the ready-file is optional.

**Conclusion:** agm-assoc is a convenience that reconciles the manifest with a
*live* pane from the inside. If `agm session import` is given (or detects) the
correct live tmux session name, agm-assoc is redundant for monitoring.

## Q2 — Minimal handoff protocol (existing conversation UUID → AGM monitoring)

Minimal sequence to make a running Claude conversation monitorable:

1. **Source the UUID** — Dispatch already holds it, or detect from the pane's
   `~/.claude/projects/<proj>/<uuid>.jsonl` (newest mtime).
2. **Source the live tmux session name** — `tmux display-message -p '#S'`
   captured *in the target pane* (authoritative), or pass it explicitly. This is
   the one value import cannot reliably infer on its own.
3. **Write the manifest** — bind `{UUID, tmuxSessionName, project, workspace,
   harness=claude-code, model=<family>}`. `RegisterSession` already does all of
   this idempotently; it returns `AlreadyTracked` instead of erroring if the UUID
   is known, so handoff is safe to retry.
4. **(Optional) ready-file** — only if a `WaitForReady` consumer is involved.
   Skip for monitor-only handoff.

Monitoring works as soon as step 3 lands and the pane named in step 2 is live.
No command needs to be *injected into* the pane for capture-pane to work — the
only reason to touch the pane is to read its `#S` (step 2).

## Q3 — Existing attachment points to reuse

| Point | Location | Reuse for ce-dbgc |
|---|---|---|
| `RegisterSession` | `internal/importer/importer.go` | **Primary.** Idempotent, non-interactive, infers project/workspace, derives name. Add harness/model params. |
| `ImportOrphanedSession` | same file | Current `agm session import` core; prompts + errors on duplicate. Less suited to live handoff. |
| `FindByUUID` / `GetSessionByUUID` | importer / `internal/dolt/sessions.go:487` | O(1) idempotency + "already monitored?" check. |
| `agm session associate` | `cmd/agm/associate.go` | Reference for UUID binding + ready-file; reuse `--auto-detect-only` re-detection logic. |
| `detection.NewDetector` | `internal/detection` | Live UUID detection / re-detection from history.jsonl. |
| `readiness.CreateReadyFile` / `WaitForReady` | `internal/readiness/wait.go` | Ready signalling, if a waiter exists. |
| `tmux.SanitizeSessionName` / `CapturePaneContent` | `internal/tmux`, `internal/monitor/tmux` | Name normalization; the actual monitor read path. |
| Manifest schema | `internal/manifest/manifest.go` | Already has `Harness`, `Model`, `Claude.UUID`, `Tmux.SessionName`. No schema change needed. |

## Recommendations for ce-dbgc

1. **Extend, don't fork.** Add `<model-family>` to the existing `import` command
   and route it through `RegisterSession` (idempotent) rather than
   `ImportOrphanedSession` (interactive/erroring). Set `Harness="claude-code"`
   and `Model=<family>` — `import.go`/`ImportOrphanedSession` currently leave
   both blank.
2. **Accept (or detect) the live tmux session name explicitly.** This is the
   single value that determines whether monitoring works; do not rely on
   sanitizing the operator-supplied display name. A `--tmux <name>` flag, or
   reading `#S` when invoked in-pane, closes the gap that would otherwise force
   an agm-assoc follow-up.
3. **Make agm-assoc optional, not mandatory.** Document that it is only needed to
   (a) reconcile a wrong tmux name from inside the pane or (b) emit a ready-file
   for a `new`-style waiter. Monitor-only handoff needs neither.
4. **Idempotency is the contract.** Re-import of a known UUID must be a no-op
   (`AlreadyTracked`), so Dispatch can fire handoff without coordination.

## Open questions for implementation

- **Model-family → harness/model mapping.** No `ModelFamily`/`InferHarness`
  helper exists today. ce-dbgc must define the mapping (e.g. `opus`/`sonnet` →
  `claude-code` + model id) or store the family verbatim in `manifest.Model`.
- **Duplicate-tmux-name collisions.** If two imported sessions sanitize to the
  same tmux name, capture-pane targets the wrong pane. Need a uniqueness check
  against live `tmux list-sessions`.
- **Does any supervisor path require the ready-file?** Confirmed only `agm
  session new` waits on it; verify no overseer/Dispatch path also blocks on it
  before declaring it optional for handoff.

## W0 requirements

- W0 (no live tmux/no ready-file, e.g. sandbox) is already handled by
  `readiness.WaitForReady`, which bounds its wait so a missing agm-assoc /
  ready-file degrades gracefully (`internal/readiness/wait.go`).
- For a monitor-only import, W0 should **not** require a ready-file at all; the
  manifest write is the only hard requirement.
