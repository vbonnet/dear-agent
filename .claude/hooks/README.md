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

## `stop-guardrail-feedback`

The WF-A guardrail feedback loop (bead `ce-vrux`; CLAUDE.md principle 2,
anti-stall). The keystone that lets deterministic guardrails close back into the
*live* agent on the laptop instead of waiting for GitHub CI.

**What it does.** Fires `Stop` and `SubagentStop`. When an agent is about to
finish a turn *and the working tree is dirty*, it runs the deterministic
guardrail bundle ([`../../scripts/guardrail-bundle.sh`](../../scripts/guardrail-bundle.sh)
— `make preflight` parity today; Semgrep/arch tests via WF-B/WF-C tomorrow). If
the bundle is **red**, it returns `{"decision":"block","reason":...}` so Claude
Code re-prompts the *same* agent with natural-language remediation, and the agent
keeps working until the guardrails are green — the "keep going until green" loop.

**It is a block, not a nudge** (the inverse of `pretool-spawn-routing`): a
failing guardrail at stop-time is exactly when a human reviewer would say "not
done — fix this first," so letting the agent stop on red would defeat the point.
But the block is *positive guidance* (principle 2) — the `reason` says what
failed, how to fix it (root cause, never suppress the check), and how to bow out.

**It cannot wedge a session** — three independent brakes:
- A per-session attempt counter capped at `DEAR_GUARDRAIL_MAX_ITERS` (default 3).
  Once exhausted it yields control back to the human with a non-blocking note
  instead of blocking again (anti-stall). The counter resets the moment the
  bundle goes green, and is keyed by `session_id` so concurrent agents don't
  share a budget (state under `${CLAUDE_STATE_DIR:-~/.claude/state}/guardrail-loop/`).
- It only engages on a **dirty** working tree, so read-only / chat / planning
  turns are never touched.
- It fails **open**: missing `jq`/`git`, unparseable input, a non-repo `cwd`, an
  absent bundle, or unwritable state all exit 0 and let the stop proceed.

**Opt out** for an interactive session with `export DEAR_GUARDRAIL_LOOP=0`.

**Scope** (principle 1): WF-A owns the *loop mechanism* only. The guardrail
*checks* live behind `scripts/guardrail-bundle.sh`, whose `run_step` list is the
extension seam WF-B (Semgrep) and WF-C (architectural tests) grow into. The hook
neither knows nor cares what the bundle runs.

**Tests.** `bats tests/bats/stop-guardrail-feedback.bats` (11 cases; drives the
bundle through its `GUARDRAIL_CMD` override so they never need the Go toolchain).
Lives under `tests/bats/`, run by `shell-matrix.yml`.

**First run.** Claude Code reviews new/changed hooks before they take effect;
approve it once when prompted.
