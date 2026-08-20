---
name: split-prs
description: Use BEFORE opening a pull request, and whenever a branch is growing past a few hundred lines or has started mixing a rename/refactor with new logic. Gives the repository's PR size budget and the order to split a change into stacked PRs. Trigger on "open a PR", "create the PR", "ready to push this up", "this diff is getting big", "should I split this", or any branch that touches both moved files and new behavior.
---

# Split PRs before you open them

Measure the branch before opening the PR:

```bash
git diff --shortstat origin/main...HEAD   # changed lines
git diff --name-only origin/main...HEAD | wc -l   # changed files
```

## The budget

**At most 400 changed lines and at most 15 changed files.**

This is a target to design toward, not a limit to creep up to. Across the 200
most recent merges to `main` the median PR was 238 changed lines and 59%
already met both numbers — it describes a normal change here.

CI's thresholds (1,000 lines / 50 files / 4 top-level areas) are **ceilings, not
targets**. A 999-line PR is four times over budget and one line under the alarm.

## Why this is not cosmetic

Evidence this rule exists: across the 200 most recent merges, 23% exceeded
1,000 changed lines, p90 was 2,251 lines, and the largest was 20,902. Every one
of the five oversized PRs merged after the size detector shipped received its
split suggestion and merged anyway, unsplit — a 100% ignore rate. See
`retrospectives/oversized-prs-advisory-signal-2026-08-19.md` in the research
repository.

Oversized PRs fail twice over. **Human review stops happening** — past a few
hundred lines a reviewer skims, and approval starts to mean "nothing obviously
alarming" rather than "I checked this." **Agent review degrades too** — a large
diff spends the reviewer's attention on bulk instead of on the few lines that
carry the risk, so specific defects get missed in exactly the PRs where a miss
costs most.

## Split in this order

1. **Mechanical** — renames, moves, generated output, formatting. Reviewable by
   confirming nothing changed but names and locations.
2. **Enabling refactor** — new seams, extracted interfaces, signature changes.
   Still no behavior change.
3. **Behavior** — the new logic, built on names and seams already on `main`.

Never put a rename and the new code that depends on it in one PR: the reviewer
cannot tell which lines moved and which are new.

## If the PR is already open: keep it as the tip

**The original PR stays as the top of the stack.** Extract the lower slices into
new PRs beneath it. Never close it and open a fresh PR for the remainder.

An open PR accumulates review threads, bot findings, CI history, and the
argument about why the change looks the way it does. That record is the most
valuable thing it owns, and it is what a later retrospective reads to
reconstruct the decision. Closing the PR discards the discussion while keeping
the code — backwards, because git keeps the code either way.

Both failure modes are in this repo's history:

- **#1307** — closed unmerged after 7 reviews, while its branch went on to be
  #1314's base and merged. The work landed; the review history did not.
- **#1133** — 22 comments and 44 reviews, split into #1301-#1304 on four new
  `stack/*` branches with one review each. #1133 stayed open but detached from
  the stack that replaced it, so its history is stranded.

```sh
# 1. cut the lower slices onto their own branches, open them bottom-up
# 2. rebase the original branch onto the topmost slice
git rebase --onto <top-slice-branch> <cut-point> <original-branch>
# 3. retarget the original PR
gh pr edit <number> --base <top-slice-branch>
```

The original keeps its number, threads, and CI history, and its diff shrinks to
the remainder as each slice lands beneath it.

Step 2 is the one part not supported end to end — `safe-rebase` has no `--onto`
mode (`ce-x2ekc`), so restack by hand rather than reaching for close-and-reopen.

Close the original **only** if the slices absorb all of it and nothing is left
at the tip; then name the superseding PRs in the closing comment so the thread
stays reachable.

## If the whole thing is already in one branch

Re-commit it in that order rather than opening one mixed PR:

```bash
git reset --soft <base>
# stage and commit the mechanical part, then the refactor, then the behavior
```

Land each step before opening the next, so **every PR is based on `main`** and
gets the full review protocol. A PR based on another PR's branch is skipped by
the five-dimension review entirely — see the caveat in
[CONTRIBUTING.md](../../../CONTRIBUTING.md#small-stacked-prs), which also covers
why restacking a descendant after a squash landing needs `--onto`.

## When it genuinely cannot be split

Some changes are atomic: a mechanical rename across many call sites, a
generated-file refresh, a vendored dependency update. Say so explicitly in the
PR description, and keep the mechanical part in its own commit so a reviewer can
skip the bulk and read the rest. "It was all one task" is not the same as
atomic — discovering two problems in one session does not make them one change.

## Verification

Before opening the PR, both must hold:

- `git diff --shortstat origin/main...HEAD` is at or under **400 changed lines**,
  and `git diff --name-only origin/main...HEAD | wc -l` is at or under **15**.
- The branch does not contain both a pure rename/move of a source file and
  net-new logic built on it. Check with
  `git diff -M --numstat origin/main...HEAD` — a record showing `0 0` with
  `old => new` is a pure move.

If either fails, split before opening rather than after. Splitting a branch you
have not yet published costs one `git reset --soft`; splitting a PR that is
already open and reviewed costs a manual `--onto` restack (see above).

When you split a PR that is already open, one more check: the **original PR
number must still be open and sitting at the top of the stack**, with its base
retargeted onto the topmost extracted slice. If you find yourself closing it and
opening a new PR for the remainder, stop — you are about to discard the review
history that makes the change auditable later.

If you open it anyway, say in the PR description which of the atomic cases
applies and why. An unexplained over-budget PR is the failure this skill exists
to prevent — the CI split-request job will ask, and the weekly audit will record
the answer.

## References

- `CONTRIBUTING.md` section "Small, stacked PRs": the budget and the split order.
- `.github/workflows/pr-size-scope.yml`: the deterministic gate and the
  split-request job that asks when it trips.
- `cmd/pr-size-audit`: the weekly sweep that reports merged offenders.
