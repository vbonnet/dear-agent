# DEAR Retro: AGM as Unified Claude Code Session Manager

**Date:** 2026-05-21
**Severity:** Medium (no incident; capability gap + one shipped fix)
**Status:** Partially resolved — MCP server fixed & deployed (PR #140);
import/remote-control verified working; two goals found architecturally
infeasible as posed and documented below.

This is the **Audit + Retro** record for the multi-part goal "Fix AGM so it can
manage Claude Code sessions end-to-end: import, unblock, archive, and expose via
MCP." It exists because the goal rested partly on an inaccurate mental model of
how AGM relates to Cowork **Desktop** sessions; that model is corrected here.

## The central finding: two session universes

There are two distinct kinds of "Claude Code session" on this machine, and AGM
was built for only one of them:

| | AGM / Claude Code **CLI** sessions | Cowork **Desktop** sessions |
|---|---|---|
| Runtime | tmux pane | Desktop app child process (no tmux) |
| Manifest | Dolt + `<workspace>/.agm/sessions/*/manifest.yaml` | `~/Library/Application Support/Claude/{local-agent-mode-sessions,claude-code-sessions}/.../local_*.json` |
| Transcript | `~/.claude/projects/*/<uuid>.jsonl` | same `*.jsonl`, keyed by the session's `cliSessionId` |
| Driven by | `agm send` / `agm answer` / `agm send select-option` → **tmux keystrokes** | the Desktop UI (and its own dispatch mechanism) |
| Status on disk | manifest `state` (hook-updated) | **none** — only `isArchived` / `lastActivityAt` (see [memory: cc-session-status-mechanisms]) |

The asymmetry that drives everything below: a Desktop session is **importable**
(its `cliSessionId` points at a real `*.jsonl`) but **not drivable** by AGM,
because AGM delivers input by sending keys to a tmux pane and a Desktop session
has none. No tmux server is even running.

## Per-task outcome & decisions

### 1. Auth/startup — *resolved (no code change needed)*
- `agm session list/new/import/answer` work **without** any token. Only
  `agm supervisor run` requires `CLAUDE_CODE_OAUTH_TOKEN`, via a presence-only
  pre-flight (`agm/cmd/agm/supervisor.go:245-253`); it also refuses if
  `ANTHROPIC_API_KEY` is set and scrubs it from the child env.
- **Decision:** The token must be minted by the user — `claude setup-token` is
  interactive and subscription-gated; it cannot be run unattended, and the
  metered `ANTHROPIC_API_KEY` is deliberately refused. Documented; not bypassed.
  User action: `claude setup-token` then export `CLAUDE_CODE_OAUTH_TOKEN`
  (or `--skip-oauth-check` for dev).

### 2. Import existing sessions — *works, verified*
- `agm session import <uuid>` imports a Claude conversation by UUID. Mapping a
  Desktop session → its `cliSessionId` → `agm session import <cliSessionId>`
  brings it under AGM. Verified on 5 sessions (the 4 video-research + the
  LinkedIn one).
- **Caveat (filed):** import resolves the *workspace* from config and writes the
  manifest into that workspace's `.agm/sessions/` — for the `oss` workspace that
  is `~/src/engram-research/.agm/`, regardless of the session's real cwd. The
  directory is git-ignored (so no history pollution) but the workspace
  attribution is wrong. Bulk auto-import would scatter mis-attributed manifests.

### 3. Auto-import via SessionStart hook — *deferred, by decision*
- No `agm session register` / hook-callable import exists. The SessionStart
  hooks (`agm/cmd/agm/install_hooks.go:135-143`) only set `state` on
  *already-tracked* tmux sessions; they cannot adopt a new session.
- **Decision:** Do **not** wire a global auto-import hook yet. `agm session
  import` is not idempotent (re-running mints a new session ID) and mis-resolves
  the workspace (see #2), so firing it on every SessionStart would create
  duplicate, mis-filed manifests. Prerequisite: an idempotent
  `agm session register <uuid>` that upserts and infers workspace from the
  session cwd. Tracked as follow-up.

### 4. Auto-enable remote-control — *already satisfied*
- It is the Claude Code setting `remoteControlAtStartup` (see
  [memory: claude-remote-control-key]). It is already `true` in both the live
  `~/.claude/settings.json` and the chezmoi source, so every new CLI session
  starts remote-controllable. No change required.

### 5. Unblock the stuck session — *infeasible via AGM (documented)*
- Session `local_52e200f5` ("Fix LinkedIn stash bug") is blocked on a two-part
  `AskUserQuestion` (edit `~/.local/bin/git-auto-sync.sh` — itself classifier-
  blocked — and how to land the branch). It is a **Desktop** session: no tmux
  pane, so `agm send select-option` (the only option-selecting path; requires a
  live pane) cannot reach it, and `agm answer` (delivers via `agm send msg` →
  tmux) cannot either.
- **Decision:** Not faked. Two compounding blockers: (a) AGM cannot drive
  Desktop sessions; (b) even the "Authorize, I'll retry" option would re-hit the
  classifier block on editing a shared scheduled script — verbal approval does
  not lift a classifier denial (see [memory: permission-classifier-vs-askuserquestion]).
  The answer must be given in the Desktop UI, and the script edit needs a
  permission rule, not just a "yes".

### 6. Archive completed sessions — *AGM side done; Desktop side held*
- **AGM:** import + `agm session archive` verified on the 4 video-research
  sessions. AGM archive only flips the manifest lifecycle flag — it does **not**
  touch worktrees, so it is safe.
- **Desktop:** `mcp__ccd_session_mgmt__archive_session` **cleans up the
  session's worktree by default**, prompts per call, and is disabled in
  unsupervised mode. Several listed sessions have **open PRs** (#137, #138, #9,
  #8) whose worktrees would be reaped.
- **Decision:** Do **not** autonomously mass-archive in Desktop. Given this
  repo's history of worktree reapers destroying unmerged work
  (see [memory: dear-agent-worktree-stop-reaper]), the per-call confirmation gate
  is correct and is left in place. Recommend the user enable
  "Auto-archive on PR close" in Settings instead.

### 7. Fix the AGM MCP server — *fixed & deployed (PR #140)*
- `agm-mcp-server` (separate stdio binary, not an `agm` subcommand) had two bugs
  that made "expose via MCP" non-functional:
  1. `registerWithClaudeCode()` was a stub that always errored, so the default
     `auto_register: true` only ever logged a warning.
  2. Tool-name drift: the get-session tool registered as `agm_get_session` while
     the SPEC/ADR-004/ARCHITECTURE/README/function name all say
     `agm_get_session_metadata`.
- **Fix:** implemented idempotent registration (preserves existing servers,
  supports flat and `mcpServers` shapes, atomic 0600 write); aligned the tool
  name to the documented contract; added tests. Built, installed to
  `~/go/bin/agm-mcp-server`, and registered user-scope via `claude mcp add` —
  now **✓ Connected**, so Cowork inherits `agm_list_sessions`,
  `agm_search_sessions`, `agm_get_session_metadata`, `agm_archive_session`,
  `agm_kill_session`, `agm_list_ops`.

## Recommended follow-ups (smallest first)
1. **Idempotent `agm session register <uuid>`** that upserts and infers
   workspace from the session's cwd — unblocks #2 (correct attribution) and #3
   (safe auto-import hook).
2. **SessionStart auto-import hook** built on (1), gated to skip already-tracked
   sessions.
3. **Desktop-session adoption** — the harder, higher-value gap: for AGM to
   *manage* (not just list) Desktop sessions it needs a non-tmux delivery
   channel. Options: an `agm answer` path that writes to a Desktop-readable
   answer file, or routing Desktop dispatch through AGM's MCP server (`agm new`)
   so sessions are tmux-backed and AGM-native from birth.
