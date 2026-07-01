---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-01
tokens: 250
title: DEAR Retro Everything
description: Every systemic defect and every seam gets a DEAR retrospective — Define, Execute, Audit, Retro — written to the knowledge base. A fix without a retro recurs.
tags: [policy, process, retrospective, dear]
---

# DEAR Retro Everything

DEAR (process level) = **Define** the defect → **Execute** the fix → **Audit**
the outcome → **Retro** the prevention. (Distinct from the code-level
Define/Enforce/Audit/Resolve hooks; see the DEAR disambiguation in the ADRs.)

## NEVER

- Fix a systemic defect and move on without a retrospective.
- Treat a recurring failure as a one-off ("just re-run it").
- Write the retro into a single agent's private memory instead of the shared
  knowledge base.

## ALWAYS

- After any seam, systemic error, or "how did this ship?" moment, write a DEAR
  retro to the knowledge base (`~/src/engram-research`, via a worktree).
- Name the root cause and a concrete prevention (a test, a gate, a constant, a
  policy), then file follow-up beads for each prevention.
- Keep it short and specific — the prevention is the point, not the narrative.

## REMINDER

The retro is where prevention lives. A fix closes one instance; the retro closes
the class. Skipping it guarantees the same failure returns on another agent's watch.
