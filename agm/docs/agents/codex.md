# Codex CLI Agent Guide

Codex in AGM is the interactive `codex` CLI harness. It runs inside tmux, uses
the same AGM session lifecycle as Claude Code and Gemini CLI, and receives
messages through the tmux pane when the Codex composer is idle.

## Setup

Install and authenticate Codex before creating a session:

```bash
which codex
codex --version
codex login
```

AGM accepts either a Codex OAuth login at `~/.codex/auth.json` or
`OPENAI_API_KEY`. The Codex harness must not use Claude OAuth tokens,
Anthropic API keys, or Claude-specific environment.

## Create

```bash
agm session new --harness=codex-cli --model=5.5 my-codex-session
```

AGM does not paste a raw `codex` command into tmux. The invoking AGM process
copies only the documented Codex environment allowlist into an owner-only,
one-shot handoff and sends a non-secret command for the absolute AGM private
executor. The executor consumes and removes the handoff before resolving the
fixed `codex` executable, constructs validated model, workdir, and sandbox
arguments, and directly replaces itself with Codex without another shell.

When Codex app-server remote control is available, AGM first creates a Codex
thread through `codex app-server`, sets the Codex thread name to the AGM session
name, stores that Codex thread id in AGM metadata, and gives the private
executor the non-secret remote resume metadata. The executor then directly
starts the matching Codex remote UI; the raw resume command and credentials
are never pasted into tmux.

Set `AGM_CODEX_REMOTE_CONTROL=0` to skip this bridge. Set
`AGM_CODEX_REQUIRE_REMOTE_CONTROL=1` to fail creation instead of falling back to
plain local Codex CLI when the bridge is unavailable.

Important launch invariants:

- model aliases are resolved through `internal/agent/models.go`
- `-C <workdir>` pins Codex to the AGM working directory
- `-s workspace-write` is the default sandbox
- before every launch/resume, AGM pre-trusts the workdir by writing
  `[projects."<workdir>"] trust_level = "trusted"` to `$CODEX_HOME/config.toml`
  (ce-cmsq). Fresh sandbox `merged/` dirs are not git repos, and an untrusted
  dir blocks the Codex TUI on an interactive trust prompt. Note
  `--skip-git-repo-check` is NOT used: the TUI and `codex resume` reject the
  flag (it is `codex exec`-only), which would brick every launch. Existing
  entries — including explicit `untrusted` — are never overwritten.
- app-server-backed starts preserve the same thread in Codex remote-control
  surfaces and in the AGM tmux pane
- the Codex child receives only the fixed allowlist; ambient Claude, Anthropic,
  Google, GitHub, Engram, OpenTelemetry, SSH-agent, and arbitrary variables
  are excluded
- credential values never appear in the tmux command, process arguments, pane
  scrollback, or debug logs

## Send

`agm send msg` treats `codex-cli` as a tmux-backed harness. When the shared
state detector sees a complete Codex composer—the initial `OpenAI Codex`
header with `/model to change` and an empty `›` cursor, or an empty post-turn
cursor paired with the structured `gpt-* · <workdir>` footer—delivery is `YES`
and AGM sends directly to the tmux pane. Typed drafts and collapsed paste chips
remain queued. Those markers must own the current pane tail: if newer shell or
process-exit output follows them, the stale composer is not sendable. A new
initial composer rendered after stale post-turn footer history is still
sendable when that newer complete structure owns the pane tail. A model name in
an echoed launch command or beside a `Working` status is not a composer and
remains queued.

Codex menus and trust prompts are not idle composers. They remain queued or
blocked rather than receiving injected prompt text.

## Resume

```bash
agm session resume my-codex-session
```

If the tmux session already exists, AGM attaches without sending commands. If
the tmux session must be recreated, AGM starts Codex with the same launch
invariants as session creation and waits for both the Codex process and a
structured idle composer to render. The complete resolved resume, including
its health read and rollback or commit, is serialized by stable AGM session ID
across direct, last-session, and bulk resume commands so a concurrent resume
cannot adopt an uncommitted pane. AGM retains a
creation-specific identity for the attempt: tmux's server-local session ID plus
a random token embedded in
a provisional creation name before being stored on that session. It then
renames the session and persists that sanitized tmux name under an opaque storage
ownership revision before submitting an optional resume prompt, and reports
ordinary prompt-delivery failures. Failed cold resumes remove only the session
whose ID and either creation marker match, including across tmux server restarts; a
same-named or ID-reusing replacement is preserved. A provisional name write is
restored only when no newer session metadata has superseded it, and its
ownership revision is released after prompt delivery succeeds. If the
post-write metadata reload fails, rollback retains that pending revision until
compensation is proven; it preserves the pane when storage cannot prove that
metadata stopped referencing the new tmux identity.

For sessions with persisted Codex metadata, AGM resumes the matching Codex
thread with `codex resume --remote unix:// <codex-thread-id>`. Older imported
or local-only sessions fall back to the saved-session behavior available from
the Codex CLI.

Codex does not support Claude runtime permission-mode cycling. AGM stores and
resumes the session shell/tmux environment, then lets Codex load its own local
state.

## Reconcile Codex-Originated Threads

Codex threads created outside AGM can be imported into AGM metadata with:

```bash
agm admin reconcile-codex          # dry-run
agm admin reconcile-codex --execute
```

This records AGM metadata only. It does not create tmux sessions, archive Codex
threads, or delete resources.

## Models

Native Codex aliases:

| Alias | Full model |
| --- | --- |
| `5.5` | `gpt-5.5` |
| `5.6` | `gpt-5.6` |
| `5.4` | `gpt-5.4` |
| `5.4-mini` | `gpt-5.4-mini` |
| `5.3-codex` | `gpt-5.3-codex` |
| `5.3-codex-spark` | `gpt-5.3-codex-spark` |

Cross-harness tier aliases are accepted where configured. For example, `opus`
and `sonnet` map to Codex's default frontier tier, and `haiku` maps to
`5.4-mini`.

## Parity Contract

Codex parity means the same AGM surfaces work for Codex CLI:

- harness validation and doctor checks
- session creation through CLI and ops-layer paths
- tmux readiness detection
- direct message delivery when idle
- queued delivery when busy or in a prompt menu
- detached resume/recreate behavior
- model alias resolution
- docs and BDD coverage

Claude-only affordances stay Claude-only unless Codex exposes an equivalent
control. Examples: `claude --resume <uuid>`, Shift-Tab permission mode cycling,
and Claude slash-command semantics.
