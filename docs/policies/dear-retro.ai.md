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
- File a "cleanup later" bead as a substitute for paring an over-fit the
  incident touched — pare it in the retro. The later never comes.

## ALWAYS

- After any seam, systemic error, or "how did this ship?" moment, write a DEAR
  retro to the knowledge base — the research repository configured in
  [`.dear-agent.yml`](../../.dear-agent.yml), via a worktree.
- Name the root cause and a concrete prevention (a test, a gate, a constant, a
  policy), then file follow-up beads for each prevention. Check for an existing
  bead on the same prevention first — +1/comment it instead of filing a
  duplicate, so repeat incidents show up as a frequency signal (and can bump
  its priority) rather than as scattered, uncounted duplicates.
- **Pare, don't defer.** If the incident touched an over-fit or idle
  scaffolding, assign it one of the six verdicts (Keep / One home / Load later /
  Turn into a check / Probation / Retire) IN the retro — see
  [harness-hygiene](harness-hygiene.ai.md). Prefer "Turn into a check" so the
  prevention is deterministic, not prose.
- Keep it short and specific — the prevention is the point, not the narrative.

## REMINDER

The retro is where prevention lives. A fix closes one instance; the retro closes
the class. Skipping it guarantees the same failure returns on another agent's watch.
