---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-01
tokens: 240
title: Autonomous Merge Policy
description: Agents review and merge their own PRs autonomously — except changes touching security, product behavior, or money, which a human merges.
tags: [policy, merge, autonomy, security]
---

# Autonomous Merge Policy

Autonomy is the default so the pipeline keeps moving; the carve-out is the
boundary that keeps consequential changes in front of a human.

## NEVER

- Autonomously merge a change touching **security**, **product behavior**, or
  **money/billing** — create those PRs as drafts and hold them for a human to
  mark ready and merge.
- Use `safe-merge --skip-review-check` or `safe-merge break-glass` in an agent
  or routine merge flow. The audited, TTY-only break-glass path is reserved for
  an explicit human emergency action.
- Merge a PR with unresolved review threads.

## ALWAYS

- For routine changes, review and merge your own PR once checks are green and
  threads are resolved — `safe-merge --pr <number>`, via the normal gate.
- Route every PR through Wayfinder V2 + `safe-pr`.
- When unsure whether something is security/product/money, treat it as yes and
  create the PR as a draft for a human.

## REMINDER

The carve-out exists because the changes that most need eyes — a guard, an auth
path, a price — are exactly the ones an over-eager agent will wave through. When
in doubt, open a draft PR and stop. Autonomy is a default, not an obligation.
