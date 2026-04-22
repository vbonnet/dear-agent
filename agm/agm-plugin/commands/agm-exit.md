---
model: sonnet
effort: low
description: Exit Claude and archive AGM session automatically
argument-hint: "{session-name}"
allowed-tools: Bash(agm session archive *), Bash(agm get-uuid *), Bash(agm admin check-worktrees *), Bash(agm get-session-name), Bash(agm send msg *), Write(~/.agm/*), Skill(engram:bow)
---

# AGM Exit

Archive the current AGM session and exit Claude.
Pre-archive verification (uncommitted changes, unmerged branch, missing tests) is handled
deterministically by `agm session archive` in Go — no LLM-based checks needed here.

## CRITICAL: Chaining Behavior

This skill invokes `/engram:bow` as a sub-skill in Step 0. After that sub-skill
completes, you MUST continue executing Steps 1–5 in the SAME response turn.
Do NOT return to the user prompt after `/bow` finishes. The entire sequence
(bow → evaluate → archive) is a single atomic operation. Treat the `/bow`
output as intermediate data — process it inline and proceed immediately.

**Step 0: Run /bow completion gate**

Before archiving, run the `/engram:bow` skill to verify session completion.

**0.1: Execute bow checks**
- Invoke: `Skill(engram:bow)`
- Capture the output and determine pass/fail status
- **IMPORTANT**: After the skill returns, do NOT stop or wait for user input.
  Continue evaluating the results in Step 0.2 immediately.

**0.2: Evaluate bow results and determine whether to BLOCK**

BLOCK the archive (stop processing, do NOT continue to Step 1) if ANY of these are detected:
- Test failures → BLOCK
- Undone tasks → BLOCK
- Broken promises → BLOCK
- Uncommitted changes → BLOCK
- Open tasks → BLOCK

If BLOCKED, display:
```
Archive blocked — bow checks failed:
{list of CRITICAL findings}

Fix the issues above, then retry /agm:agm-exit
```

WARNING-level issues (e.g., missing docs, extra branches) do NOT block — note them and continue to Step 1.

**0.3: Report bow findings to orchestrator**
- If an orchestrator session exists, report findings:
  - Run: `agm send msg orchestrator "bow-gate: {PASS|FAIL} — {summary of findings}"`
- If no orchestrator: skip silently

**0.4: Continue to exit steps**
- If bow PASSED (no CRITICAL findings): proceed IMMEDIATELY to Step 1 below.
  Do NOT output a summary and wait. Do NOT return control to the user.
  The exit sequence continues in this same response.

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
- [ ] `/engram:bow` passed — no CRITICAL findings
- [ ] All bow findings addressed (not deferred to "next session")
- [ ] Retrospective written, committed, and pushed
- [ ] `/agm:agm-exit` ran (this skill) — archive confirmed
- [ ] Session no longer appears in `agm session list`

**If any item is incomplete, do NOT run `/agm:agm-exit`.** Fix first, then exit.
