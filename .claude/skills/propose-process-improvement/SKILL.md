---
name: propose-process-improvement
description: Use whenever you notice process friction mid-task or have a fix for how the team/agents work, e.g. "this keeps happening", "our process failed here", "I have a fix for how we work", "we keep hitting X". Captures the problem or the fix as one Beads item instead of a log line or a dropped comment, without stopping the current task.
---

# Propose a process improvement

Never let a process observation evaporate into a chat message or a mental
note. The DEAR retro policy exists precisely because unlogged friction
recurs: `~/src/engram-research/retrospectives/flywheel-death-pattern-2026-06-27.md`
records three supervisors sitting idle for 7+ hours with zero alerts reaching
a human, and `~/src/engram-research/retrospectives/2026-08-11-results-surfacing-blindness.md`
records a broken read surface that misled two different consumers for
roughly 12 days before the fix PR (#1212) even opened. Both were observable
long before they were P0/P1 incidents; nobody had a lightweight way to log
"this looks wrong" without dropping the current task.

This skill is that lightweight way. It does not replace a full DEAR retro
(see `docs/policies/dear-retro.ai.md`) for a seam or systemic error that
already happened; it is for catching friction *before* it becomes one, per
[anti-stall](../../docs/policies/anti-stall.ai.md): track the blocker in Beads
and keep working, don't stop to write a report.

## Workflow

1. State the problem or the proposed fix in one sentence: what's broken or
   friction-causing, and what a fix would look like if known.
2. Search for an existing bead on the same prevention first:

   ```sh
   bd --db ~/beads/context-engine/.beads --dolt-auto-commit on list --status=open --json | grep -i "<keyword>"
   ```

3. If a matching bead exists, add a `+1`/comment instead of a duplicate, so
   repeat incidents show up as a frequency signal that can bump its priority:

   ```sh
   bd --db ~/beads/context-engine/.beads --dolt-auto-commit on comment <id> \
     "+1: observed again in <task/session context>, <one-line detail>"
   ```

4. Otherwise create a new bead with a testable acceptance criterion, not a
   vague "looks better":

   ```sh
   bd --db ~/beads/context-engine/.beads --dolt-auto-commit on create \
     "<short title>" \
     --description="<goal>. Acceptance: <observable check>." \
     --type=task --priority=<0-4>
   ```

5. Continue the task you were on. Filing the bead is the deliverable; it is
   not a reason to stop and wait.

## Verification

- A bead ID was printed by `bd create` or `bd comment`. That is the deliverable — the task you were on continues.
- If `bd list --status=open` was run and a matching bead already existed, a `+1` comment was added instead of a duplicate.
- The session you were in has not stalled: you returned to the original task immediately after filing.

## Traps this skill kills

- **Silent duplication.** Filing a fresh bead every time the same friction
  recurs hides the frequency signal that would otherwise justify raising its
  priority. Search before create, every time.
- **Vague acceptance.** "Process should be better" is not a bead a future
  session can close. State the observable check.
- **Treating this as a stop-the-world report.** This skill is anti-stall by
  design: log it, keep going. A full incident write-up belongs in a DEAR
  retro, not here.
