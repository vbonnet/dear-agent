# Why: Definition of Done

## The principle
"Done" that stops before merge leaves the repo in a half-state that the next
agent has to reverse-engineer. Autonomous agents optimize for whatever we call
the finish line — if the finish line is "PR open," we accumulate open PRs and
divergent branches instead of a moving `main`.

## Real failure cases (this repo)
- **Unmerged interdependent clusters.** Multiple times, related PRs were all left
  open and interdependent (e.g. a session-lifecycle cluster of 4+ PRs), none
  merged, each blocking the others — work that was "done" by every measure except
  the one that counts.
- **Stale worktrees.** Dozens of worktrees accumulated for branches whose work was
  "finished" but never merged/reaped — each a half-state an agent could wander into.
- **Admin-bypass merges laundering red.** When merge *was* forced, `--admin` pushes
  landed red/unverified changes — the opposite failure: "merged" without "verified."
  Done requires both: merged AND green AND verified, by the normal path.

## How to apply
- Treat merge → deploy → verify as part of the task, not follow-up.
- Close the bead only on merge, with the PR link in the closing note.
- Blocked from merging? Escalate the blocker up the chain; keep the pipeline
  moving on the next unblocked item. Parking silently is not "done."
- Never reach "done" via `--admin`, force-pushing protected/default branches,
  or `--no-verify`. Force-pushing non-default PR branches after a clean rebase
  is allowed; prefer `--force-with-lease`.

See also: [autonomous-merge](autonomous-merge.why.md), [broken-windows](broken-windows.why.md).
