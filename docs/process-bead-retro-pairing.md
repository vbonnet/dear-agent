# Process: File Each DEAR Retro as a Bead + Doc Pair

**Status:** Active
**Source:** DEAR retro `2026-06-15-dogfooding-audit-retro.md`, Tier 0 action T0.2 (`ce-x9ul.1`)

## Why

We already write DEAR retros — it is the one mandate we pay willingly. But
they land as standalone docs in `engram-research/retrospectives/`
with no matching bead. That makes the retro corpus a floating file
collection: you can `grep` it, but you cannot query it through the bead
graph (by label, parent epic, status, or relationship to the work that
caused the incident).

Fix is pure habit, ~0 cost: **create a bead first, write the doc, link the
bead to the PR.** One `bd create` call per retro converts the corpus into a
queryable graph. Companion to T0.1 (`ce-5vje`, one bead per task).

## When to create a retro bead

Create a bead **any time a DEAR retro doc is written**:

- Incident / postmortem (CI red streak, broken merge, credential hang)
- Daily ops or dogfooding audit
- Milestone or epic review
- Supervisor / agent failure analysis

Rule of thumb: if it goes in `engram-research/retrospectives/`, it gets a bead first.

## Naming convention

Pick the dated slug once; use it for both the doc filename and the bead
title so the two are trivially cross-referenced.

| Artifact      | Format                                                   | Example                                                     |
| ------------- | -------------------------------------------------------- | ----------------------------------------------------------- |
| Doc filename  | `engram-research/retrospectives/<YYYY-MM-DD>-<slug>.md`  | `engram-research/retrospectives/2026-06-20-ci-cascade.md`   |
| Bead title    | `retro: <YYYY-MM-DD>-<slug> — <short summary>`           | `retro: 2026-06-20-ci-cascade — CI red 6h`                  |

## Bead creation command

There is no built-in `retro` issue type (defaults are
`bug|feature|task|epic|chore|decision`), so use `--type task` with a
`dear-retro` label — matching the existing `ce-x9ul.1` bead.

```sh
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on create \
  "retro: 2026-06-20-ci-cascade — CI red 6h" \
  --type task \
  --priority 1 \
  --labels dear-retro,retro \
  --description "DEAR retro for the 2026-06-20 CI cascade. Doc: engram-research/retrospectives/2026-06-20-ci-cascade.md"
```

Required fields:

- **title** — `retro: <date>-<slug> — <summary>` (see naming convention)
- **`--type task`** — no custom `retro` type configured
- **`--priority`** — `1` (P1) for incidents/audits; `2` for routine reviews
- **`--labels dear-retro,retro`** — `dear-retro` makes the corpus queryable;
  add topic labels (`ci`, `dogfooding`, …) as useful
- **`--parent <epic>`** — attach to the relevant epic when one exists

## PR linking

Link the bead and the PR **both ways** so either side reaches the other:

1. **PR body** — include the bead ID, e.g. `Resolves T0.2 (ce-x9ul.1)`.
2. **Bead comment** — after the PR opens, record the URL:

   ```sh
   bd --db ~/beads/context-engine/.beads --dolt-auto-commit on comment <bead-id> \
     "PR: https://github.com/vbonnet/dear-agent/pull/<num>"
   ```

3. **Doc header** — name the bead ID in the retro doc's `Source:` line.

## Definition of Done

A retro bead is closed only when **all three** hold:

1. Retro doc is committed to `engram-research/retrospectives/`.
2. Bead and PR are linked both ways (PR body + bead comment).
3. PR is merged to `main`.

Then:

```sh
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on close <bead-id>
```

## Backfill guidance

For the existing floating retro docs in `engram-research/retrospectives/`:

- **Do not** mass-create beads retroactively — low value, high churn.
- **Do** create a retroactive bead when you next *touch* a floating retro
  (cite it, link it, or write a follow-up). Use the doc's original date in
  the slug; note `(backfill)` in the description.
- Going forward, the bead-first habit applies to **all new** retros.

## Minimal template

```sh
DATE=2026-06-20; SLUG=ci-cascade
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on create \
  "retro: ${DATE}-${SLUG} — <summary>" \
  --type task --priority 1 --labels dear-retro,retro \
  --description "DEAR retro. Doc: engram-research/retrospectives/${DATE}-${SLUG}.md"
# → write engram-research/retrospectives/${DATE}-${SLUG}.md
# → open PR, body cites the bead ID
# → bd --db ~/beads/context-engine/.beads --dolt-auto-commit on comment <bead-id> "PR: <url>"
# → on merge: bd --db ~/beads/context-engine/.beads --dolt-auto-commit on close <bead-id>
```
