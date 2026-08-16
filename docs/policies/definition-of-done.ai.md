---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-01
tokens: 230
title: Definition of Done
description: Done means merged to main, deployed where applicable, and verified in the real system — not "code written", "tests pass", or "PR open".
tags: [policy, process, done, merge]
---

# Definition of Done

Work is not done until it is live. Code on a branch, a green local test, or an
open PR are checkpoints, not completion.

## NEVER

- Call a task done at "code written", "tests pass", or "PR opened".
- Close a tracking bead before the change is merged to `main`.
- Leave a mergeable PR unmerged and consider the work complete.

## ALWAYS

- Done = **merged to `main`** + **deployed** (where a deploy step exists) +
  **verified** in the real system.
- Drive each change through to merge; resolve review threads; then, once deployed
  (where applicable) and verified, close the bead with a link to the merged PR.
- If something blocks the merge, escalate it — do not silently park it as "done."

## REMINDER

Done means merged, deployed where applicable, and verified in the real system.
Unmerged work rots, diverges from `main`, and becomes the next agent's confusing
half-state. The last mile — merge, deploy, verify — is the job, not an optional
epilogue.
