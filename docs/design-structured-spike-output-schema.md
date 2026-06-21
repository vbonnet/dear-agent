# Design Spike: Typed/Structured Output Schema for Spike Beads

**Bead:** ce-t8kn · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21

## Problem

Spike outputs are free-form markdown. A consumer — a supervisor triaging the
backlog, a next-bead creator deriving W0 requirements, or the W0.5 gate
([[ce-4h06]]) reading a confidence score — must parse prose to recover the
options evaluated, the recommendation, the tradeoffs, and the W0 list. That
parse is error-prone (heading variance, missing sections) and slow (an LLM round
trip per consumer). The fields that downstream automation needs are *latent* in
the document instead of *addressable*. We want a typed envelope that makes them
machine-readable without abandoning the human-readable doc.

## 1. Required Fields

A spike output is the markdown doc **plus** a structured sidecar with these
fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `schema_version` | string | yes | e.g. `spike.v1` — enables migration (§4) |
| `bead_id` | string | yes | the spike bead this output closes |
| `problem_statement` | string | yes | one paragraph; mirrors doc §Problem |
| `options[]` | array | yes | ≥1; each `{id, summary, pros[], cons[], evidence[]}` |
| `recommended_option` | string | yes | an `options[].id`; `null` only if escalating |
| `tradeoffs[]` | array | no | `{axis, by_option{}}` — the comparison table, normalized |
| `w0_requirements[]` | array | yes | each `{text, acceptance, files[], effort}` — feeds the impl bead |
| `confidence` | object | yes | the [[ce-90si]] block: `{score, band, dimensions{}, scorer, ts}` |
| `companion_beads[]` | array | no | referenced `[[bead-id]]`s, extracted as data |
| `open_questions[]` | array | no | residual unknowns; feeds the unknown-unknowns dimension |
| `doc_path` | string | yes | path to the human-readable markdown |

`options[]` and `w0_requirements[]` are the load-bearing fields: the first is
what a confidence reviewer re-scores for completeness, the second is what a
next-bead creator turns into an implementation bead with zero re-parsing.

## 2. Format

Options considered:

- **Frontmatter in the markdown doc.** One file, human + machine co-located,
  diffable. But YAML frontmatter is awkward for nested arrays and easy to drift
  from the prose below it.
- **Separate schema file in repo** (`docs/spikes/<bead>.spike.json`). Clean
  validation, CI-checkable, but a second artifact to keep in sync with the doc.
- **bd metadata JSON** (`spike.output` field on the bead). Queryable by
  supervisors directly via `bd`, co-located with the bead that owns it — but bd
  metadata is not ergonomic to author by hand or review in a diff.
- **trail.jsonl entry.** Immutable audit record, append-only — good as a
  *mirror*, wrong as a source of truth (not editable on revision).

**Recommendation: YAML frontmatter as the authoring source of truth, mirrored
to bd metadata at spike close.** The frontmatter keeps the structured data in
the same file the worker is already writing (one artifact, one PR diff,
reviewable inline); a close-time hook validates it against the schema and writes
the canonical JSON to `bd` metadata (`spike.output`) so supervisors and the
W0.5 gate query one queryable place. The trail entry is the immutable audit
copy. This matches the storage split the confidence spike already assumes
([[ce-90si]] §2): frontmatter = author, bd metadata = query, trail = audit.

```yaml
---
schema_version: spike.v1
bead_id: ce-t8kn
problem_statement: Spike outputs are free-form; consumers must parse prose.
options:
  - id: frontmatter
    summary: Typed YAML frontmatter in the markdown doc.
    pros: [one artifact, inline-diffable, co-located with prose]
    cons: [nested arrays awkward, can drift from body]
    evidence: [docs/design-confidence-gated-spike-output.md §2]
  - id: bd-metadata
    summary: Structured JSON on the bead.
    pros: [directly queryable by supervisors]
    cons: [poor authoring/diff ergonomics]
    evidence: []
recommended_option: frontmatter
tradeoffs:
  - axis: queryability
    by_option: {frontmatter: medium, bd-metadata: high}
  - axis: authoring-ergonomics
    by_option: {frontmatter: high, bd-metadata: low}
w0_requirements:
  - text: Add a close-time validator that lints frontmatter against spike.v1.
    acceptance: A doc missing a required field fails spike close with a clear error.
    files: [tools/spike-validate, schemas/spike.v1.json]
    effort: M
confidence:
  score: 78
  band: HIGH
  dimensions: {option_completeness: 4, evidence_strength: 3, w0_specificity: 4, unknown_unknowns: 3}
  scorer: worker:ce-t8kn
  ts: 2026-06-21
companion_beads: [ce-90si, ce-4h06]
open_questions:
  - Should tradeoffs[] be derived from options[] rather than authored twice?
doc_path: docs/design-structured-spike-output-schema.md
---
```

## 3. Production Mechanism

Three candidates: (a) **schema-constrained tool call** — the worker emits the
object via a `StructuredOutput` tool whose schema is `spike.v1`, validated at
the call layer so the model retries on mismatch; (b) **post-hoc extraction** —
a second agent parses the finished markdown into the schema; (c) **required
section headers** — fixed `##` headings the worker must fill.

**Recommendation: schema-constrained tool call as primary, post-hoc extraction
as the backfill path.** The constrained call is the only option that guarantees
the fields exist at authoring time rather than hoping a parser recovers them; it
also lets the worker self-score confidence in the same structured emission
([[ce-90si]] §2 stage 1). Post-hoc extraction is retained solely to migrate the
existing free-form corpus (§4). Required headers are too weak alone — they
constrain shape, not content, and still need parsing.

## 4. Backward Compatibility

Existing free-form docs have no frontmatter and must keep working.

- **Version-gated.** Absence of `schema_version` ⇒ treat as `spike.v0`
  (free-form). Consumers branch on the field; v0 docs are read as prose exactly
  as today.
- **Opt-in label.** A `structured-output` bead label marks spikes expected to
  carry v1 frontmatter; the close-time validator only fires when the label is
  present, so unlabeled and historical spikes are never blocked.
- **Lazy migration.** A post-hoc extractor (mechanism §3b) can backfill v1
  frontmatter for high-value historical spikes on demand — e.g. the ~15–20 beads
  the confidence calibration ([[ce-90si]] §3) wants to retro-score — without a
  big-bang rewrite.

## 5. W0 Requirements for the Implementation Bead

1. `schemas/spike.v1.json` — versioned JSON Schema for the envelope above;
   `schema_version` mandatory and matched on read.
2. A `StructuredOutput` spike-emission path bound to `spike.v1`, with
   validate-and-retry; worker self-score ([[ce-90si]]) emitted in the same call.
3. A close-time validator (label-gated on `structured-output`) that lints the
   frontmatter and mirrors canonical JSON to `bd` metadata (`spike.output`) +
   a trail entry, written atomically.
4. A post-hoc extractor to backfill v1 frontmatter for selected v0 docs.
5. Consumer read helper that resolves frontmatter → bd metadata → v0-prose
   fallback, so one accessor serves supervisors, next-bead creators, and W0.5.
6. Acceptance: a labeled spike missing a required field fails close; a v0 doc
   still closes unchanged; the W0.5 gate reads `confidence.score` without parsing
   prose.

## Companion Beads

[[ce-90si]] confidence-gated decision function (consumes `confidence` block) ·
[[ce-4h06]] W0.5 gate (reads the score) · [[ce-kky0]] recursive decomposition ·
[[ce-ynyb]] spike pattern adoption.
