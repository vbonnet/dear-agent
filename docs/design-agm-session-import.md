# Design: `agm session import <uuid> <model-family>`

**Bead:** ce-dbgc (design spike) · **Status:** Proposed · **Date:** 2026-06-21
**Supersedes context in:** [`spike-agm-session-import-handoff.md`](./spike-agm-session-import-handoff.md) (ce-akkp)

## Context

Dispatch needs to hand off an already-running Claude Code conversation to a
supervisor for monitoring without spawning a fresh session. The input is a
conversation UUID plus a model family; the output is an AGM-registered session
that a supervisor can poll. An `agm session import <uuid>` command already
exists (`agm/cmd/agm/import.go` → `importer.ImportOrphanedSession`), but it
prompts for a name, errors on duplicates, and never records harness/model. This
ADR specifies the `<model-family>` extension and the non-interactive path.

## Decision

Extend the existing command to `agm session import <uuid> <model-family>`,
routing through `importer.RegisterSession` (idempotent, non-interactive) rather
than `ImportOrphanedSession` (interactive, errors on duplicate). The command
does **not** spawn a tmux pane and does **not** inject `/agm:agm-assoc`.

### 1. Session identity model

An AGM session is a **Dolt DB record** (`manifest.Manifest`, keyed by
`SessionID`), carrying `Claude.UUID`, `Tmux.SessionName`, `Context.Project`,
`Workspace`, `Harness`, `Model`, and `State` fields. Of these, an imported
conversation natively has only the **UUID** and a **project path** (derivable
from the transcript). The tmux pane, the `State`/heartbeat fields, and the
ready-file are runtime artifacts produced by a *live* pane's hooks — an import
creates the DB record without them. Import's job is to mint the record;
liveness is layered on later (or never, for transcript-only monitoring).

### 2. UUID lookup

Transcripts live at `~/.claude/projects/<encoded-project-path>/<uuid>.jsonl`
(slashes in the path encoded as dashes). `importer.InferProjectPath(uuid)`
already performs the reverse lookup — scanning project dirs for `<uuid>.jsonl`
and recovering the project path from the containing directory.
`ExtractMetadataFromHistory` then reads `last activity` and project path. No
Claude API call is needed; lookup is purely local-filesystem.

### 3. The `<model-family>` parameter

`RegisterSession` today leaves `Manifest.Harness` and `Manifest.Model` blank
(`importer.go:208`). The family argument fills both. It is validated and
resolved via the existing `agent` package:

- `agent.ValidateModel("claude-code", family)` rejects unknown families.
- `agent.ResolveModelFullName("claude-code", family)` maps the alias to a full
  id (`opus`→`claude-opus-4-8[1m]`, `sonnet`→`claude-sonnet-4-6[1m]`, etc.).

The family determines three things: (a) the `Manifest.Model` recorded for
display/audit; (b) the **cost basis** — pricing lookup keys on model family
(`context_detector` pricing table); (c) the `--model` flag *if* the session is
later resumed into a pane. For monitor-only import it is metadata + cost basis;
it does not spawn anything.

### 4. agm-assoc integration

**Not required.** `/agm:agm-assoc` is a thin plugin command whose only job is to
run `agm session associate <name> --create` *from inside a live pane* to (a)
read the real tmux `#S` and (b) emit the ready-file. Import is detached/headless
— there is no live TUI to send a slash command to, and routing a deterministic
DB write through the plugin→LLM→Bash→mode chain is the known-fragile path
(memory: `agm-assoc-plugin-dependency`, bead ce-o1sg). Import writes the
UUID→manifest binding directly, in-process, bypassing that chain entirely.

agm-assoc adds value only when (a) the imported tmux name must be reconciled
against a live pane from the inside, or (b) a `WaitForReady` consumer needs the
ready-file. Monitor-only handoff needs neither.

### 5. Monitoring path

Supervision **reads a pane**, it does not attach to a process: the monitor calls
`tmux capture-pane -p -t <Tmux.SessionName>` (`internal/monitor/tmux`). The only
binding it needs is `Tmux.SessionName` → a live pane of that name. For a
conversation already running in a pane, pass `--tmux <name>` (or capture `#S`
in-pane) so the recorded name matches reality. The richer `State`/heartbeat
fields are written by the conversation's own Claude Code hooks — present only
while a pane runs; a detached import shows static state until resumed.

### Sequence

```
Dispatch                agm session import          importer/agent           Dolt DB        Supervisor
   │                          │                          │                      │               │
   │  import <uuid> <family>  │                          │                      │               │
   │  [--tmux <name>]         │                          │                      │               │
   │─────────────────────────>│                          │                      │               │
   │                          │ ValidateModel(family)    │                      │               │
   │                          │─────────────────────────>│                      │               │
   │                          │ InferProjectPath(uuid)   │ scan ~/.claude/      │               │
   │                          │   + ExtractMetadata      │   projects/*/uuid.jsonl              │
   │                          │<─────────────────────────│                      │               │
   │                          │ FindByUUID(uuid) ────────┼─────────────────────>│               │
   │                          │   AlreadyTracked? ───────┼── no ────────────────│               │
   │                          │ RegisterSession{UUID, tmux, project,            │               │
   │                          │   Harness=claude-code, Model=resolve(family)}   │               │
   │                          │──────────── CreateSession(manifest) ───────────>│               │
   │<───── SessionID ─────────│                          │                      │               │
   │                          │                          │   poll capture-pane(Tmux.SessionName)│
   │                          │                          │                      │<──────────────│
```

## Edge cases

- **Already attached elsewhere** — `FindByUUID`/`GetSessionByUUID` (indexed,
  O(1)) returns the existing record; import returns `AlreadyTracked` as an
  idempotent no-op. Re-import is safe for Dispatch to fire without coordination.
- **UUID not local** — `InferProjectPath` finds no `<uuid>.jsonl`; fail fast
  with a clear "no local transcript for UUID" error. (No remote fetch in W0.)
- **Session finished** — a finished conversation still has a transcript, so the
  record imports fine, but no live pane exists; `capture-pane` will miss.
  Monitoring degrades to transcript-only until/unless resumed.
- **Duplicate tmux name** — two imports may sanitize to the same
  `Tmux.SessionName`, pointing capture-pane at the wrong pane. Add a uniqueness
  check against `tmux list-sessions` and suffix on collision.

## W0 requirements (implementation bead)

1. **CLI:** add the `<model-family>` positional to `import.go` (`ExactArgs(2)`);
   add `--tmux <name>` and keep `--name`/`--workspace`.
2. **Wiring:** route through `RegisterSession`, not `ImportOrphanedSession`;
   extend `RegisterSession` to accept `harness`/`model` and set them on the
   manifest (currently hardcoded blank at `importer.go:208`).
3. **Validation:** call `agent.ValidateModel` + `ResolveModelFullName` before
   the DB write.
4. **No schema change** — `Manifest` already has `Harness`, `Model`,
   `Claude.UUID`, `Tmux.SessionName`.
5. **No agm-assoc protocol change** — import does the binding directly; document
   agm-assoc as optional reconciliation only.
