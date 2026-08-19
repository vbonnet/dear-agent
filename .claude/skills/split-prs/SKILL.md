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
