---
name: verify-receipts
description: Use before closing a bead, ending an Execute phase, or accepting any "done" claim, e.g. "is this actually done", "verify the claim", "mark this complete". Checks a completion claim against real receipts, including receipts that should exist but don't, instead of accepting narration.
---

# Verify by receipts

"The write succeeded" is not evidence; reading the state back is. Two
documented incidents show what accepting narration instead of receipts costs:
`~/src/engram-research/retrospectives/2026-08-11-results-surfacing-blindness.md`
records a session-state write that always "succeeded" while silently writing
nothing, undetected for roughly 12 days, and
`~/src/engram-research/retrospectives/flywheel-death-pattern-2026-06-27.md`
records three supervisors that looked alive (tmux panes responded) while
having received zero real instructions for 7+ hours. In both cases the
failure was an *absence* the existing checks weren't built to see, not a
loud error.

This skill checks both halves: the receipts that should be present, and the
receipts that should exist but don't.

## Workflow

1. **Positive receipts** — for the specific claim under review, confirm the
   artifact directly, don't take a summary's word for it:
   - Commit reachable from the target branch: `git log --oneline <base>..<sha>`
   - PR actually merged, not just opened: `gh pr view <n> --json state,mergedAt`
   - Test/build output that ran, not just "tests should pass":
     re-run the narrowest relevant command (`make preflight`,
     `make test-affected`, or the specific package's tests)
   - Bead reflects the claimed state: `bd --db ~/beads/context-engine/.beads --dolt-auto-commit on show <id>`

2. **Absent-event checks** — the failure modes above were invisible to
   checks that only look for errors. Check for the expected event's
   *absence*:
   - PR open with no state change: `gh pr view <n> --json updatedAt,state,statusCheckRollup`
     — stale beyond ~48h or carrying a red check is itself a finding
     (`pr-merge-blockers` diagnoses the exact blocker; run it, don't guess)
   - Session/supervisor record stuck at spawn:
     `agm session get <id>` — `created_at == updated_at` after meaningful
     wall time means nothing happened, not that nothing was wrong
   - A dispatched task with no corresponding bead state change since it was
     claimed started

3. For every claim that fails either check, or every absence found, file or
   `+1` a bead the same way `propose-process-improvement` does — search
   first, dedup, then create with an observable acceptance criterion. A
   verify pass that finds nothing to file is a valid, completed outcome; one
   that finds something and doesn't file it is not.

4. Only report "done" once every claimed artifact in this checklist actually
   resolved to real state, not once the checklist was merely run.

## Traps this skill kills

- **Trusting a status line.** "Pushed", "merged", "tests pass" are claims,
  not receipts, until re-checked against the actual system.
- **Only checking for errors.** A quiet failure produces no error to catch;
  it produces the *absence* of a change you expected. Check for that
  explicitly, per step 2.
- **Verifying and not filing.** Finding a gap and not turning it into a bead
  reproduces the exact "cleanup later never comes" pattern
  `docs/policies/dear-retro.ai.md` warns against.
