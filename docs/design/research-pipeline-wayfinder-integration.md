# Research Pipeline × Wayfinder — Incorporate or Delegate?

**Status:** Recommendation
**Date:** 2026-07-19
**Author:** research-pipeline-wayfinder-assessment session
**Related:**
[research-pipeline SKILL (dotfiles PR #51)](https://github.com/vbonnet/dotfiles/pull/51),
[engram-research PR #143 — first dogfood run](https://github.com/vbonnet/engram-research/pull/143),
[`wayfinder/README.md`](../../wayfinder/README.md),
[`wayfinder/PHASES.md`](../../wayfinder/PHASES.md),
[`wayfinder/cmd/wayfinder-session/internal/beads/SPEC.md`](../../wayfinder/cmd/wayfinder-session/internal/beads/SPEC.md)

## Problem

A multi-model pipeline — provider-routed ingest → goal-oriented research →
independent cross-model verification and planning → decomposition into
sized beads → dark-factory execution — has now run end-to-end once for
real (engram-research PR #143) and been formalized as a standalone Claude
Code skill, `research-pipeline` (dotfiles PR #51, refined alongside this
doc). Its core discipline is **cross-model handoffs** (no model grades its
own homework), **provider routing with a receipt**, and a **human review
gate before execution commits resources**.

dear-agent already owns a decomposition/phase-gate engine — Wayfinder, a
9-phase SDLC methodology (`CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC →
PLAN → SETUP → BUILD → RETRO`) with a canonical Beads adapter that files
tasks non-interactively at the PLAN phase. Two architectures are on the
table:

1. **Incorporate** — fold the research-pipeline's stages into Wayfinder as
   new phases (or phase variants), so a single system owns both.
2. **Delegate** — keep `research-pipeline` as its own orchestrator skill,
   and have it call into Wayfinder only for the piece Wayfinder already
   does well (structured decomposition, sized beads with dependency
   sequencing).

This doc grounds the decision in what Wayfinder actually is (not what its
name suggests it might be), and recommends one architecture over the
other.

## What Wayfinder actually is

Reading `wayfinder/README.md`, `wayfinder/PHASES.md`, and the Beads
adapter SPEC directly (not from memory — the V1/V2 phase model has
changed shape at least once, and `wayfinder/PHASES.md` exists specifically
to correct stale assumptions about phase count):

- **A single-project, single-session SDLC workflow**, not a general task
  orchestrator. `/wayfinder:start "desc"` creates one project directory
  with a `WAYFINDER-STATUS.md` and walks it through 9 gated phases.
  Designed for "multi-phase project (>1 day effort)" per its own
  `README.md` — explicitly **not** for a task under an hour.
- **PLAN is the real decomposition mechanic**, backed by
  `wayfinder/cmd/wayfinder-session/internal/beads` — an adapter with five
  EARS requirements (`WAYFINDER-BEADS-01..05`) covering DB-path
  resolution, availability checks, non-interactive `bd create` with no
  shell interpolation, and empty-title rejection. This is the piece worth
  reusing: a tested, safe, canonical path from "task breakdown" to filed
  beads.
- **`wayfinder-decompose` is a different tool** — a Python script that
  splits one **XL charter** into several **sub-project charters** by
  platform/layer/concern keyword detection (e.g. "this charter mentions
  both `frontend` and `backend` → split into two sub-projects"). It has
  nothing to do with turning a verified research plan into sized beads
  with acceptance criteria. The original `research-pipeline` SKILL.md
  conflated these two ("Sol / wayfinder" in one box); this doc's
  companion PR corrects that.
- **RESEARCH and DESIGN phases are scoped narrowly.** Per `PHASES.md`,
  Wayfinder's RESEARCH phase means "search for existing solutions
  (build/buy/adapt)" — for a project whose problem is already charter'd.
  It is not "ingest and cite an external source" and does not have an
  ingestion mechanic (no provider routing, no `provenance.json`
  equivalent).
- **Multi-persona validation exists but isn't the same discipline as
  cross-model handoff.** `README.md` lists "automatic domain expert
  detection (Security, ML, etc.) for design reviews" as a DESIGN-phase
  feature. That's closer to research-pipeline's cross-model rule than
  anything else in Wayfinder, but it's opt-in persona simulation within
  one session, not a hard requirement that phase N+1 be reviewed by a
  model that didn't author phase N's artifact.

None of this is a criticism of Wayfinder — it's a different tool solving a
different-shaped problem (guided single-project SDLC rigor) than
research-pipeline (turning an arbitrary external source into verified,
adversarially-checked, executable work, often stopping after stage 2).

## Where the two pipelines actually overlap

| research-pipeline stage | Wayfinder equivalent | Real overlap? |
|---|---|---|
| Stage 1 — Ingest (provider routing + receipt) | none | No. Wayfinder has no source-ingestion concept. |
| Stage 2 — Research (`deep-research`) | PROBLEM / RESEARCH phases | Superficial. Wayfinder's RESEARCH assumes the problem is already charter'd; research-pipeline's Stage 2 is what produces the material a charter would later be written from. |
| Stage 3 — Independent verify + plan | DESIGN / SPEC phases | Partial. DESIGN's multi-persona review is the closest analog to "a model that didn't author this checks it," but it's not enforced as cross-*model* independence. |
| **Stage 4 — Decomposition into sized beads** | **PLAN phase + Beads adapter** | **Real, direct overlap.** This is genuinely the same mechanic — task breakdown into a canonical tracker with dependency sequencing. |
| Stage 5 — Execution (Codex / `safe-pr`) | BUILD phase | Partial. BUILD is TDD implementation with state-machine enforcement inside a Wayfinder session; research-pipeline's Stage 5 is plain Codex-picks-up-a-bead-and-runs, no session required. |

Exactly one stage (decomposition) maps cleanly onto exactly one Wayfinder
phase (PLAN). Everything else is either absent from Wayfinder (ingestion,
provider routing, the human review gate as research-pipeline defines it)
or a looser analog that would need real design work to unify (RESEARCH,
DESIGN, BUILD).

## Candidates

### 1. Incorporate — fold research-pipeline stages into Wayfinder

Add ingestion as a pre-CHARTER phase, treat Stage 2/3 as RESEARCH/DESIGN
proper, keep PLAN as-is for Stage 4, and let BUILD absorb Stage 5.

| Criterion | Verdict |
|---|---|
| Reuses PLAN + Beads adapter | ✅ Already true without incorporating anything — it's a skill dependency, not a merge. |
| Fits Wayfinder's project shape | ❌ Wayfinder assumes a >1-day, single-project charter from the start. Most research-pipeline runs stop at Stage 2 (a curiosity question) — forcing every one-off research question through a 9-phase project session is the wrong default per Wayfinder's own "don't use when: single-file change, requirements already clear" guidance, and "requirements already clear" doesn't even apply here — there usually isn't a charter yet. |
| Cross-model independence as a first-class rule | ❌ Would require adding a new enforcement primitive to Wayfinder's phase-gate machinery (reviewer-≠-author, not just "a persona reviewed this"). Real work, not a config flag. |
| Provider-routing + receipt (Stage 1) | ❌ No existing Wayfinder concept to extend; would be new from scratch either way. |
| Blast radius | ❌ V1→V2 phase consolidation already happened once (see `PHASES.md`) specifically because two near-duplicate phases (`S4`/`D4`, `S9`/`S10`/`S8`) turned out not to be independently useful in practice. Adding a new INGEST phase for a use case (external source → plan) that a large fraction of Wayfinder users will never hit repeats that mistake in the other direction. |
| Two-repo problem | ⚠️ research-pipeline's Stage 1/2/3 artifacts belong in `engram-research` (the private KB), not `dear-agent`. Wayfinder sessions live in-repo (`WAYFINDER-STATUS.md` at project root). Incorporating would need a new cross-repo session model Wayfinder doesn't have today. |

**Verdict: reject.** The one real overlap (decomposition) doesn't justify
merging two systems with different session models, different repos, and
different session-length assumptions. It would make Wayfinder heavier for
its existing (larger) user base of >1-day project work, to serve a
minority of research-pipeline runs that actually reach Stage 4.

### 2. Delegate — standalone SKILL calls Wayfinder for decomposition only

Keep `research-pipeline` as its own orchestrator (already true per PR
#51). Stage 1-3 run entirely outside Wayfinder, in `engram-research`.
Stage 4, when the decomposition is large enough to need phased
sequencing, drives a Wayfinder session's PLAN phase (which files beads
through the tested Beads adapter) instead of reimplementing sequencing
logic or a second `bd create` path. Small decompositions skip Wayfinder
entirely and call `bd create` directly — full session overhead isn't
justified for 2-3 beads. Stage 5 stays plain Codex/`safe-pr`, with the
option (not requirement) to wrap it in a full Wayfinder BUILD phase if the
initiative genuinely warrants that level of gating.

| Criterion | Verdict |
|---|---|
| Reuses PLAN + Beads adapter | ✅ Same reuse as candidate 1, without the merge cost. |
| Fits Wayfinder's project shape | ✅ Only large decompositions touch Wayfinder at all; research-only runs never create a Wayfinder session. |
| Cross-model independence as a first-class rule | ✅ Already enforced at the skill level (research-pipeline's own Rules section), no Wayfinder changes needed. |
| Provider-routing + receipt (Stage 1) | ✅ Owned entirely by research-pipeline; no dependency on Wayfinder either way. |
| Blast radius | ✅ Zero changes to Wayfinder's phase model, EARS requirements, or session format. |
| Two-repo problem | ✅ Non-issue — Stage 1-3 artifacts stay in `engram-research`; only the Wayfinder session (if invoked) lives in `dear-agent`, same as any other Wayfinder-managed project. |
| Precedent | ✅ This is already what PR #51's SKILL.md does (`"via wayfinder:wayfinder skill's structured decomposition"`), and what this doc's companion PR sharpens (correcting the `wayfinder-decompose` confusion, adding the size-based routing rule). |

**Verdict: this is already the right architecture, now made precise.**

## Recommendation

**Delegate, don't incorporate.** `research-pipeline` stays a standalone
orchestrator skill; Stage 4 calls into Wayfinder's PLAN phase + Beads
adapter for decompositions that need phased sequencing, and calls `bd
create` directly for small ones. This was already the directional choice
in PR #51 — the value of this assessment is confirming it against
Wayfinder's actual (not assumed) architecture, and correcting one real
error (conflating the Beads adapter with the unrelated `wayfinder-decompose`
XL-charter splitter) before it became a running mistake in a live pipeline.

### When to revisit

1. **If research-pipeline runs start reliably reaching Stage 4-5 with
   >1-day scope** (not the common case today — most runs are one-off
   research or small application plans), a tighter Wayfinder integration
   for Stage 5 (running BUILD as a gated session per bead, not just
   `safe-pr`) becomes worth designing.
2. **If Wayfinder gains a native "reviewer must differ from author"
   phase-gate primitive** for other reasons (this is a real gap — see
   below), research-pipeline's Stage 3/4 cross-model rule could migrate
   from skill-level convention to a Wayfinder-enforced property, which
   would be a genuine strengthening. Not worth building solely for this
   pipeline today.

### What a deeper integration would take, if ever pursued

The most promising direction is **not** merging the pipelines — it's
generalizing Wayfinder's existing "multi-persona validation" (currently
DESIGN-phase-only, persona-based) into a first-class **cross-model
reviewer-independence gate** usable at any phase: the gate records which
model authored an artifact and refuses to accept a review from that same
model. That would benefit any Wayfinder project wanting "no model grades
its own homework" discipline, not just research-pipeline-originated ones.
Scope: a new gate-check primitive in
`wayfinder/cmd/wayfinder-session/internal/validator/` (same family as the
existing `spec_ears_gate.go`, `doc_quality_gate.go`), model-identity
plumbing through the session status file, and a decision on which phases
require it (DESIGN and PLAN are the obvious first candidates, given
that's where research-pipeline's own Stage 3/4 loop already lives). This
is a multi-day Wayfinder-core change, not a research-pipeline one — filed
as a follow-up idea, not started here: **ce-2pj5**.

## Decisions

| Decision | Rationale |
|---|---|
| Keep `research-pipeline` as a standalone skill, not a Wayfinder phase set. | Different session model (cross-repo, often stops at Stage 2), different length assumption (Wayfinder targets >1-day projects). |
| Stage 4 large decompositions route through Wayfinder's PLAN phase + Beads adapter. | Real, direct mechanic overlap — reuse a tested non-interactive `bd create` path instead of building a second one. |
| Stage 4 small decompositions call `bd create` directly, no Wayfinder session. | Full 9-phase session overhead isn't justified for a handful of beads. |
| Do not add an INGEST phase to Wayfinder's V2 model. | Repeats the V1→V2 consolidation mistake in reverse — a phase most Wayfinder projects would never use. |
| Do not build a cross-model reviewer-independence gate in Wayfinder right now. | Real gap, genuinely valuable, but multi-day core work with no urgent driver beyond this one pipeline. Tracked as follow-up idea **ce-2pj5**. |

## Open questions

1. **Should research-pipeline's Stage 5 ever wrap a bead in a full
   Wayfinder BUILD-phase session** (state-machine-enforced TDD) instead
   of plain Codex + `safe-pr`? Today's answer is "only if the bead is
   individually big enough to warrant it" — left to the executor's
   judgment, not mandated by the skill. Revisit if Stage 5 defects trace
   back to missing BUILD-phase rigor.
2. **Does `wayfinder-decompose` (the XL-charter platform/layer/concern
   splitter) need a name change or a doc pointer to prevent the same
   confusion this doc corrects from recurring?** Out of scope here; flag
   for whoever next touches that script.

## References

- [research-pipeline SKILL — dotfiles PR #51](https://github.com/vbonnet/dotfiles/pull/51)
- [engram-research PR #143 — first dogfood run, including the Stage-4
  verify-loop this doc and PR #51's companion refinement are grounded
  in](https://github.com/vbonnet/engram-research/pull/143)
- [`wayfinder/README.md`](../../wayfinder/README.md)
- [`wayfinder/PHASES.md`](../../wayfinder/PHASES.md)
- [`wayfinder/cmd/wayfinder-session/internal/beads/SPEC.md`](../../wayfinder/cmd/wayfinder-session/internal/beads/SPEC.md)
- [`docs/skill-verification-criteria.md`](../skill-verification-criteria.md)
