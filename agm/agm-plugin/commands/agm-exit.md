---
model: sonnet
effort: low
description: Exit Claude and archive AGM session automatically
argument-hint: "{session-name}"
allowed-tools: Bash(agm session archive *), Bash(agm get-uuid *), Bash(agm admin check-worktrees *), Bash(agm get-session-name), Bash(agm send msg *), Bash(git status *), Bash(git log *), Write(~/.agm/*)
---

# AGM Exit

Archive the current AGM session and exit Claude.
Pre-archive verification (uncommitted changes, unmerged branch, missing tests) is handled
deterministically by `agm session archive` in Go — no LLM-based checks needed here.

## CRITICAL: Chaining Behavior

All steps must run in the SAME response turn without returning control to the user.
The entire sequence (completion gate → archive) is a single atomic operation.

**Step 0: Inline completion gate**

Run the following bash checks directly. Do NOT call any external skill — the
engram:bow skill is unavailable (its source repo ~/src/engram does not exist on
this machine). These inline checks replace it.

**0.1: Check for uncommitted changes**

Run: `git -C <project-dir> status --porcelain 2>/dev/null`

Replace `<project-dir>` with the current session's project directory. If the
session is not associated with a git repo, skip this check.

BLOCK if output is non-empty (uncommitted or untracked files exist).

**0.2: Check for unmerged branches**

Run: `git -C <project-dir> log --oneline origin/main..HEAD 2>/dev/null | head -10`

BLOCK if output shows commits that haven't been pushed/merged.

**0.3: Evaluate and determine whether to BLOCK**

BLOCK the archive (stop processing, do NOT continue to Step 1) if ANY of these are detected:
- Uncommitted changes (Step 0.1 output non-empty) → BLOCK
- Unmerged branch commits (Step 0.2 output non-empty) → BLOCK

If BLOCKED, display:
```
Archive blocked — completion gate failed:
{list of CRITICAL findings}

Fix the issues above, then retry /agm:agm-exit
```

WARNING-level issues (extra branches, missing docs) do NOT block — note them and continue.

**0.4: Report gate result to orchestrator**
- If an orchestrator session exists, report:
  - Run: `agm send msg orchestrator "completion-gate: {PASS|FAIL} — {summary of findings}"`
- If no orchestrator: skip silently

**0.5: Continue to exit steps**
- If gate PASSED: proceed IMMEDIATELY to Step 1 below.
  Do NOT output a summary and wait. Do NOT return control to the user.

**Step 1: Determine session name**

Check argument first, then auto-detect via agm. Do NOT call tmux directly.

- If $ARGUMENTS is non-empty, use that as SESSION_NAME
- Else run: `agm get-session-name` — if exit 0, use output as SESSION_NAME
- Else: show "Could not detect session name" and "Usage: /agm:agm-exit {session-name}", then stop

Do NOT use `echo`, `printf`, `printenv`, `tmux`, `touch`, or bash conditionals.
Output text directly in your response — NEVER via bash echo/printf commands.
Handle results in your reasoning layer.

**Step 2: Verify AGM association**

- Run: `agm get-uuid "{SESSION_NAME}"`
- If exit code ≠ 0: show "Session not associated with AGM — run /agm:agm-assoc first", then stop

**Step 3: Check for orphaned worktrees**

- Run: `agm admin check-worktrees --session "{SESSION_NAME}"`
- If exit code = 1: show the output (lists orphaned worktrees) and ask the user
  whether to clean them up with `agm admin cleanup-worktrees --session "{SESSION_NAME}"` or skip
- If exit code = 0: continue to Step 4

**Step 4: Set exit-gate marker**

- Use the Write tool to create an empty marker file: `Write(file_path="~/.agm/exit-gate-{SESSION_NAME}", content="")`
- Do NOT use `touch` — it is blocked by the pretool-bash-blocker hook.
- This marker authorizes the `pretool-exit-gate` hook to allow the archive command.
  Without it, direct `agm session archive` calls are blocked as a safety gate.

**Step 5: Archive session**

- Run: `agm session archive "{SESSION_NAME}" --async --cleanup-worktrees`
- If exit code ≠ 0:
  - Show the error output (includes specific verification failures from Go checks)
  - If output contains "Cannot archive": show each failure and suggest fixes
  - Show fallback: "Manual exit: /exit then agm session archive {SESSION_NAME} --force"
  - Stop

On success, display:
```
Async archive started — a background reaper will send /exit,
wait for the pane to close, clean up worktrees, and archive
the session automatically. Nothing more to do.
```

## Exit Completion Checklist (Reference)

This checklist summarizes what "successfully archived" means. The orchestrator
uses this to verify session lifecycle completion. Workers should self-check
against this before running `/agm:agm-exit`.

- [ ] All code changes committed and pushed
- [ ] Tests written and passing (no "deferred" or "TODO test")
- [ ] Inline completion gate passed (no uncommitted changes, no unmerged commits)
- [ ] Retrospective written, committed, and pushed
- [ ] `/agm:agm-exit` ran (this skill) — archive confirmed
- [ ] Session no longer appears in `agm session list`

**If any item is incomplete, do NOT run `/agm:agm-exit`.** Fix first, then exit.
