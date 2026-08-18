# Engineering Policies — canonical source

Repo-committed engineering principles that **every agent on every harness** must
follow. These are the source of truth — not any single agent's memory (a sticky
note on one engineer's desk does not scale to agents on other models or machines).

Each policy is a pair:
- `<name>.ai.md` — the policy. Terse, actionable, `NEVER` / `ALWAYS` / `REMINDER`.
- `<name>.why.md` — the reasoning, real failure cases, and how to apply it.

Loaded by all harnesses through the `AGENTS.md` `@import` chain
(`CLAUDE.md`, `CODEX.md`, `GEMINI.md`, `AGY.md`, `OPENCODE.md` all import `AGENTS.md`,
which points here). Also discoverable via `engram guidance search`.

| Policy | One-line |
|---|---|
| [anti-stall](anti-stall.ai.md) | Continue through known work, accept empty results, and stop only at explicit boundaries. |
| [broken-windows](broken-windows.ai.md) | Deprecated code is the next agent's precedent — delete it completely, in the same change. |
| [harness-hygiene](harness-hygiene.ai.md) | Earn every mechanism continuously; WHAT+WHY not HOW; page over-fits now, defer over-harness; never delete-because-it's-long. |
| [dear-retro](dear-retro.ai.md) | Every seam, every systemic error gets a DEAR retro. A fix without a retro recurs. |
| [definition-of-done](definition-of-done.ai.md) | Done = merged to main, deployed, verified in prod. Not "code written" or "PR open". |
| [wayfinder-v2-canonical](wayfinder-v2-canonical.ai.md) | Wayfinder V2 (9 phases) is canonical. V1 (13 phases) is dead. |
| [autonomous-merge](autonomous-merge.ai.md) | Agents review+merge their own PRs — except security/product/money, which a human merges. |

## Adding or changing a policy
Policy changes define agent behavior — treat as **governance**: open a Wayfinder V2
PR and hold for **human merge review** (see [autonomous-merge](autonomous-merge.ai.md)).
When a policy supersedes older guidance elsewhere, delete the old copy in the same
change (see [broken-windows](broken-windows.ai.md)) — do not leave two sources of truth.
