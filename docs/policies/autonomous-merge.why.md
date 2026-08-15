# Why: Autonomous Merge Policy

## The principle
For an autonomous fleet to make progress, agents must own the full routine
lifecycle — draft, ready, review, merge — for their own work; waiting on a
human for every typo fix, or for every draft to be flipped ready, does not
scale. But the same autonomy applied to a security guard, an auth path, a
billing change, or the policy that governs agent behavior itself is how a
subtle, high-blast-radius mistake lands unreviewed. The carve-out
(security / product / money / agent-governance / agent-control-surface →
human) draws that line. Outside the carve-out, the human-only gate was on the
draft→ready flip, not on the change itself — 2026-08-14, Valentin authorized
removing that gate for ordinary PRs: agents may mark ready, review, and merge
non-sensitive PRs themselves.

## Real failure cases (this repo)
- **Security fix silently reverted.** A verbatim god-file-split PR showed
  MERGEABLE + green while dropping a `ValidateModel` security fix twice
  (2026-05-16, 2026-05-17). An autonomous merge of a "clean refactor" would have
  reverted a security control — exactly what the carve-out is for.
- **Admin-bypass merges laundering red.** PRs merged via `--admin` over pending
  required checks; `new-from-merge-base` lint made bypassed red look green on
  `main`. This is why the policy forbids `--admin`, force-pushes to protected
  branches, and `--no-verify`.
- **Fail-open guard + config-path collision.** Security-guard changes look like
  routine cleanups but change what the guard actually blocks — hold for a human.

## How to apply
- Ask: does this touch a security control, user-visible product behavior,
  money/billing, an agent-governance document (e.g. `docs/policies/`), or an
  agent control surface (auth, quota, notification, merge-policy)? If yes →
  open the PR as a draft, do NOT mark it ready or merge it, flag for human
  review.
- Otherwise → the PR is agent-owned end to end: mark it ready for review
  yourself once it's otherwise mergeable, review it, resolve threads, and
  merge autonomously once green via `safe-merge --pr <number>`.
- Never reach for `--admin`, force-pushes to `main`/`master`/default,
  `--no-verify`,
  `safe-merge --skip-review-check`, or `safe-merge break-glass` in a routine
  agent flow — fix the cause or escalate. The audited, TTY-only break-glass
  path remains human-operated.
- Force-pushing non-default PR branches is allowed when rebasing stale work;
  prefer `--force-with-lease`.

See also: [definition-of-done](definition-of-done.why.md).
