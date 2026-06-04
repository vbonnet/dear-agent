# Project hooks

Repo-scoped Claude Code hooks, wired in [`../settings.json`](../settings.json).
Project hooks **merge additively** with the user's global `~/.claude/settings.json`
hooks (they don't replace them), so these run *alongside* the machine-level
safety guards, only when a session's project dir is dear-agent.

## `pretool-spawn-routing`

Dogfooding routing nudge (Beads `ce-qgf`; CLAUDE.md §Dogfooding, principle 6).

**What it does.** Fires `PreToolUse` on `Bash` and the Cowork scheduled-task MCP
tool. When it detects an attempt to spawn a **new top-level agent session/task**
outside our own orchestration — a raw `claude` / `claude-code` / `cowork` CLI
session, or `mcp__scheduled-tasks__create_scheduled_task` — it injects a positive
reminder pointing at the AGM/VROOM path. Every task not on AGM/VROOM is a missing
data point for the self-improvement flywheel.

**It is a nudge, not a gate** (principle 2). The hook emits *only*
`additionalContext`; it never returns a `permissionDecision`, so it can neither
block a call nor auto-approve one. The normal permission flow is untouched, and
it fails **open** — any unparseable input exits 0 silently.

**Scope is deliberately narrow** (principle 1):
- The `agm` command is intentionally *not* matched — it is the right path.
- The in-session `Agent`/`Task` subagent tool is *not* matched — a subagent runs
  inside the parent AGM session, so it never escapes the mesh.
- Read-only `claude` subcommands (`mcp`, `config`, `doctor`, `--version`, …) are
  skipped — they don't start a session.
- Spawns at the human/Desktop layer (opening Cowork by hand) are outside any
  in-session hook's reach. This is a best-effort net for the *programmatic* path,
  not a complete gate. See [`AGENTS.why.md`](../../AGENTS.why.md).

**Tests.** `bats tests/bats/pretool-spawn-routing.bats` (13 cases; asserts the
nudge fires on spawns, stays silent on the AGM path and ordinary commands, and
never emits a `permissionDecision`). Lives under `tests/bats/` — the repo's
sanctioned shell-test path, run by `shell-tests.yml` and `shell-matrix.yml`.

**First run.** Claude Code reviews new/changed hooks before they take effect;
approve it once when prompted.
