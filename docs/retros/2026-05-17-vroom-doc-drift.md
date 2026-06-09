# DEAR Retro: VROOM Documentation Drift

**Date:** 2026-05-17
**Severity:** Medium (no incident, but core architecture was mis-documented
across ~12 files, including the alignment docs that govern agent behavior)
**Status:** Resolved — VROOM relocated/rewritten, `CONTEXT.md` created as the
vocabulary source of truth, referencing docs swept.

This is the **Audit + Retro** half of the DEAR loop for the work
"fix and relocate the VROOM architecture docs". The **Define** half is
[docs/adr/ADR-002](../adr/ADR-002-vroom-execution-architecture.md).

## Define

Correct the VROOM architecture documentation: it described the wrong model
(five roles Verifier/Requester/Orchestrator/Overseer/Meta-Orchestrator + a
lexicographic value evaluator) and was misfiled under `agm/` even though VROOM
is higher-level than AGM. Establish a single vocabulary source of truth and
make the AGM / VROOM / DEAR / Wayfinder relationship explicit.

## Execute

- New `CONTEXT.md` at repo root: normative vocabulary + the four-framework
  relationship + a "Known Terminology Collisions" register.
- New `docs/adr/ADR-002-vroom-execution-architecture.md` (top-level: signals
  "above AGM"), recording the corrected 3-supervisor model and its trade-offs.
- `agm/docs/adr/ADR-020`…`025` reduced to redirect stubs (history + numbering
  preserved; dangling `DEAR-PROTOCOL.md` / `orchestrator-mission.md` links
  removed).
- Swept: `docs/alignment/{MISSION,VALUES,VISION,GOALS}.md` (repointed
  `adr_ref`, removed the dead "Verifier role"), `ARCHITECTURE.md`,
  `docs/adr/ADR-022`, `agm/docs/meta-orchestrator-mission.md`,
  `agm/docs/adr/README.md`.
- Dated audit/retro records got non-destructive forward banners (point-in-time
  history is preserved, not rewritten — per the append-only audit-trail value).
- **ADR prune (scoped):** ran a repo-wide ADR audit against the
  hard-to-reverse + surprising + real-trade-off test —
  [docs/audits/2026-05-17-adr-inventory-prune.md](../audits/2026-05-17-adr-inventory-prune.md).
  Executed only the in-theme, unambiguous subset here: deleted
  `docs/adr/ADR-008` (obsolete Draft) and the exact-duplicate
  `engram/internal/telemetry/enrichment/ADR-001`; added DEAR-collision
  disambiguation banners to `docs/adr/ADR-010/011/018`. The ~80 nested
  code-local ADR changes are deferred to follow-up surgical PRs (FU-1…FU-6 in
  the audit) — bundling them here would have been an unreviewable mega-diff
  that violates the repo's own surgical-commit rule.
- **Consolidated the dual top-level ADR directories:** `docs/adrs/` (plural)
  merged into `docs/adr/` (singular, the conventional name); all inbound
  references repointed (Go doc comments, ROADMAP, CONTEXT.md). ADR numbers
  unchanged. Nested per-package `*/docs/adrs/` dirs intentionally untouched.

## Audit

- `grep` confirms no remaining dangling `DEAR-PROTOCOL.md` /
  `orchestrator-mission.md` references, no stale `adr_ref: ADR-020`, and no
  surviving "five-role" *description* outside the explicitly-corrective
  mentions.
- Docs-only change: Go build/test/lint surface is unaffected.

## Retro — process gap and the systemic fix

**Root cause (architecture level, not instruction level):** there was **no
vocabulary single-source-of-truth**, and the architecture was spread across six
independently-editable per-role ADRs. Nothing forced them to agree, so they
drifted together away from intent and dragged the alignment docs with them via
`adr_ref` frontmatter. This is the same class of failure as the research-artifact
leaks that motivated `.dear-agent.yml`: a rule with no anchor that agents could
silently diverge from.

**Systemic fixes shipped here:**
1. `CONTEXT.md` is now the anchor; docs are required to defer to it for
   definitions, and disagreements must be reconciled, not coexist.
2. The single architectural decision lives in one ADR; role descriptions are
   vocabulary, not five drifting ADRs.

**Follow-ups proposed (route through the Meta-Orchestrator — roadmap authority;
not added to the roadmap here):**
- `pkg/vroom` decision-trail topics still encode the superseded role enum
  (`vroom.decision.evaluated` "Verifier"). Renaming exported constants is a
  breaking change → needs its own ADR/PR.
- "DEAR" has three live meanings (process loop / workflow-engine code hooks /
  backlog phase prefix). Recommend renaming the code-level lifecycle so it
  stops shadowing the process loop.
- `agm/docs/adr/README.md` index is stale (stops at ADR-011) and needs a full
  reindex.

(The "two top-level ADR directories" follow-up was resolved in this PR — see
the Execute section.)
