# Spike: Pocock skill-description token optimization (ce-0jqa)

Applies Matt Pocock's model- vs user-invocable skill split. Skill descriptions
are loaded into context up-front so the model can decide *when* to fire a skill.
A skill that the model can never auto-invoke does not need that decision context —
its description is pure display text and should be a single sentence.

- **Model-invocable** (auto-triggered): needs a rich `description` with TRIGGER /
  DO-NOT-TRIGGER guidance so the model knows when to fire and when not to.
- **User-invocable only** (explicit `/slash-command`, `disable-model-invocation:
  true`): the user already named the skill, so a 1-sentence display description
  suffices. Any TRIGGER/DO-NOT-TRIGGER prose is dead weight.

## Inventory & classification

| Skill | File | Class | Action |
|-------|------|-------|--------|
| beads | `.agents/skills/beads/SKILL.md` | model-invocable | keep (rich triggers) |
| wayfinder | `wayfinder/skills/wayfinder/SKILL.md` | model-invocable | keep (already 1 line) |
| review-architecture | `tools/spec-review/skills/review-architecture/SKILL.md` | model-invocable | keep (rich triggers) |
| review-spec | `tools/spec-review/skills/review-spec/SKILL.md` | model-invocable | keep (rich triggers) |
| review-adr | `tools/spec-review/skills/review-adr/SKILL.md` | model-invocable | keep (rich triggers) |
| create-spec | `tools/spec-review/skills/create-spec/SKILL.md` | **user-invocable** (`disable-model-invocation: true`) | **trimmed** |
| scan-health | `agm/agm-plugin/skills/scan-health/SKILL.md` | user-invocable | already 1 line |
| agm-list | `agm/internal/surface/skills/agm-list.md` | user-invocable (slash cmd) | already 1 line |
| agm-search | `agm/internal/surface/skills/agm-search.md` | user-invocable (slash cmd) | already 1 line |
| agm-status | `agm/internal/surface/skills/agm-status.md` | user-invocable (slash cmd) | already 1 line |
| vroom orchestrator/overseer/meta-orchestrator/protocol | `cmd/vroom-dispatch/skills/*.md` | not skills (no frontmatter; operational docs loaded directly) | n/a |

## Key finding

`create-spec` is the only skill that is both **user-invocable only**
(`disable-model-invocation: true`) **and** carries a full model-decision
description. The model can never auto-invoke it, so the TRIGGER / DO-NOT-TRIGGER
prose was never consulted — it only cost context tokens.

## Savings

`create-spec` description: **305 → 63 chars** (−242 chars, ≈ −60 tokens, ~79% on
this field). All other user-invocable skills were already minimal, so no further
trims were warranted. The remaining skills are genuinely model-invocable and
correctly retain rich descriptions.

## Recommendation going forward

When authoring a new skill, set `disable-model-invocation: true` only when the
skill is meant to be typed by a human, and pair it with a one-sentence
description. Reserve TRIGGER / DO-NOT-TRIGGER prose for skills the model must
decide to fire on its own.
