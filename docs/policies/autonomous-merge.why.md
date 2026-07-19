# Why: Autonomous Merge Policy

## The principle
For an autonomous fleet to make progress, agents must merge their own routine
work — waiting on a human for every typo fix does not scale. But the same
autonomy applied to a security guard, an auth path, or a billing change is how
a subtle, high-blast-radius mistake lands unreviewed. The carve-out
(security / product / money → human) draws that line.

## Real failure cases (this repo)
- **Security fix silently reverted.** A verbatim god-file-split PR showed
  MERGEABLE + green while dropping a `ValidateModel` security fix twice
  (2026-05-16, 2026-05-17). An autonomous merge of a "clean refactor" would have
  reverted a security control — exactly what the carve-out is for.
- **Admin-bypass merges laundering red.** PRs merged via `--admin` over pending
  required checks; `new-from-merge-base` lint made bypassed red look green on
  `main`. This is why the policy forbids `--admin`/`--force`/`--no-verify`.
- **Fail-open guard + config-path collision.** Security-guard changes look like
  routine cleanups but change what the guard actually blocks — hold for a human.

## How to apply
- Ask: does this touch a security control, user-visible product behavior, or
  money/billing? If yes → open the PR, do NOT merge, flag for human review.
- Otherwise → merge autonomously once green and all threads are resolved via
  `safe-merge --pr <number>`.
- Never reach for `--admin`, `--force`, `--no-verify`,
  `safe-merge --skip-review-check`, or `safe-merge break-glass` in a routine
  agent flow — fix the cause or escalate. The audited, TTY-only break-glass
  path remains human-operated.

See also: [definition-of-done](definition-of-done.why.md).
