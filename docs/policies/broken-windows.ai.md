---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-01
tokens: 240
title: Broken Windows
description: Deprecated code is the next agent's precedent — remove it completely, in the same change, never leaving two versions to coexist.
tags: [policy, engineering, cleanup, deprecation]
---

# Broken Windows

Your hack, your half-migration, your commented-out "old way" — the next agent
finds it, assumes it is precedent, and extends it. Two implementations then
diverge forever.

## NEVER

- Leave two implementations of the same thing coexisting after a replacement.
- Comment out or `// deprecated`-tag old code and move on "to delete later."
- Ship a migration that adds the new path without deleting the old one.
- Leave docs, flags, or entrypoints describing a thing that no longer exists.

## ALWAYS

- When you replace or deprecate code, delete the old implementation in the SAME
  change, so the new one is the only version an agent can find.
- Migrate anything worth keeping first, then remove the source completely
  (entrypoints, prompts, dead flags, stale docs included).
- If you must leave a seam temporarily, file a bead and link it at the seam.

## REMINDER

Deprecated-but-present code WILL be found and built upon. "Remove it later"
becomes "two diverging systems." Cleanliness is not aesthetic — it is the only
thing that keeps autonomous agents converging instead of forking the codebase.
