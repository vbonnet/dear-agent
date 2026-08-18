---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-01
tokens: 280
title: Autonomous Merge Policy
description: Agents mark ready, review, and merge their own PRs autonomously — except changes touching security, product behavior, money, agent governance, or agent control surfaces, which a human marks ready and merges.
tags: [policy, merge, autonomy, security]
---

# Autonomous Merge Policy

Autonomy is the default so the pipeline keeps moving; the carve-out is the
boundary that keeps consequential changes in front of a human.

## NEVER

- Autonomously merge a change touching **security**, **product behavior**,
  **money/billing**, **agent governance** (`docs/policies/`, alignment docs,
  and other documents that define agent behavior), or an **agent control
  surface** (auth, quota, notification, or merge-policy controls) — create
  those PRs as drafts and hold them for a human to mark ready and merge.
- Mark a carve-out PR ready for review or arm auto-merge on it. The
  draft→ready transition for these categories is human-only.
- Use `safe-merge --skip-review-check` or `safe-merge break-glass` in an agent
  or routine merge flow. The audited, TTY-only break-glass path is reserved for
  an explicit human emergency action.
- Merge a PR with unresolved review threads.

These categories are enforced in code by `internal/mergeloop.DefaultSensitiveGlobs`
(`Classify` returns `blocked-policy`). A category added here without a matching
glob is not enforced — the PR still classifies green and reaches `safe-merge`.

## ALWAYS

- Diagnose a non-merging or unclear PR with `pr-blockers <number>` BEFORE any
  other investigation. It names the exact blocker and exact fix from GitHub's
  merge state (mergeStateStatus, required checks, review threads including
  outdated ones). Guessing at a merge blocker is a defect
  (`.claude/skills/pr-merge-blockers`).
- For routine (non-carve-out) changes, mark your own draft ready for review,
  review it, and merge it once checks are green and threads are resolved —
  `safe-merge --pr <number>`, via the normal gate. The draft→ready transition
  is agent-owned for ordinary PRs; it is not a human-only step by default.
- Route every PR through Wayfinder V2 + `safe-pr`.
- When unsure whether something is security/product/money/governance/control-
  surface, treat it as yes and leave the PR as a draft for a human.

## REMINDER

The carve-out exists because the changes that most need eyes — a guard, an auth
path, a price, a policy — are exactly the ones an over-eager agent will wave
through. When in doubt, open a draft PR and stop. Autonomy is a default, not an
obligation.
