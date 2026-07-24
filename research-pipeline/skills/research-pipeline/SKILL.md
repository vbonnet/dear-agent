---
name: research-pipeline
description: >
  Orchestrates the staged pipeline for turning an external source (a talk,
  video, article, paper) into verified, executable work — provider-routed
  ingestion, goal-oriented research, independent cross-model verification and
  planning, decomposition into sized beads, and dark-factory execution. Use
  when the user wants to ingest a source (especially a YouTube video or talk)
  and turn it into a concrete plan or beads for this codebase, wants a second
  model to fact-check research before acting on it, or says things like
  "research this and turn it into a plan", "verify this with an independent
  model and plan the application", or "decompose this into beads for Codex".
  Do NOT use for a single-shot research question with no downstream codebase
  action, or for grabbing a transcript with no further processing.
---

# Research Pipeline

Five stages, each owned by an existing skill or an independent model. This
skill's job is provider-routing discipline, staged handoffs between
DIFFERENT models (never let one model mark its own homework), a human review
gate before execution, and consistent storage so the research trail stays
navigable.

```
  ingest              research              verify+plan            decompose            execute
 ┌───────────┐   ┌───────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────┐
 │ provider- │──▶│ goal-oriented │──▶│ independent model:  │──▶│ independent model:  │──▶│  Codex   │
 │ routed    │   │ web research  │   │ adversarial fact-   │   │ re-verify plan,     │   │ dark-    │
 │ ingest    │   │ (fan out,     │   │ check + codebase-   │   │ split into sized    │   │ factory  │
 │           │   │  cite, verify)│   │ grounded plan       │   │ beads, file via     │   │ via      │
 │           │   │               │   │                     │   │ `bd create` or a    │   │ safe-pr  │
 │           │   │               │   │                     │   │ wayfinder PLAN phase│   │          │
 └───────────┘   └───────────────┘   └──────────┬──────────┘   └──────────┬─────────┘   └──────────┘
                                                  │                        │
                                                  ├──▶ loop back to fix ◀──┤  (repeat until reviewer
                                                  │    if reviewer blocks │   verdict is unconditional
                                                  │                        │   SHIP — see "Verification
                                                  └───── HUMAN REVIEW GATE ┘   loops" below)
                                                    (present before decomposing
                                                     / before execution starts)
```

## Workflow

1. **Ingest** — route to the right provider for the source type, keep a
   provenance receipt (below).
2. **Research** — goal-oriented web research: fan out, fetch, adversarially
   verify claims, cite everything.
3. **Verify + plan** — hand off to a model that did not author the research;
   it re-checks hard claims and writes a codebase-grounded application plan.
4. **Decompose** — a third independent model reviews the plan and splits it
   into sized beads with testable acceptance criteria.
5. **Execute** — beads move through normal `safe-pr`/`safe-merge` execution.

## Not every task needs all five stages

| You have…                                            | Start at                              |
|-------------------------------------------------------|----------------------------------------|
| A source (video/article) and nothing else            | Stage 1 (ingest)                       |
| Raw source already saved, need it understood          | Stage 2 (research)                     |
| A research doc you're not sure is trustworthy         | Stage 3 (independent verify + plan)    |
| A verified plan, need it actionable                   | Stage 4 (decompose into beads)         |
| Sized beads with a DoD, ready to build                | Stage 5 (Codex / safe-pr execution)    |
| Just "what does this video say"                       | Stop after Stage 1-2 — don't chain     |

Chain forward only as far as the concrete downstream goal requires. A
one-off curiosity question stops at Stage 2. Only chain to Stage 4/5 when
the destination is genuinely a codebase change.

## Stage 1 — Ingest: route to the right provider, keep a receipt

**Rule: name the provider that's actually right for the source type — never
silently let whichever harness is running absorb the task.** YouTube/video →
a video-capable provider first (e.g. Gemini). If the running harness can't
reach that provider natively (no tool, confirmation denied, headless mode
blocks it), fall back — but the fallback and the reason MUST be recorded,
not silently swallowed.

Store artifacts in the operator's designated private research/notes
repository — not this repo; dear-agent is the shared product repo, and
research artifacts are working material tied to one operator, not the
product. Ask the user which repository and path convention apply if this
isn't already established for the session; do not assume a specific
repository name or hardcode a machine path. Inside that repository, use:

```
research/<YYYYMMDD-HHMMSS>-<sourcetype>-<id>/
  metadata.json      # title, author/speaker, source URL, date, duration
  provenance.json     # extract_method, and if the named provider was NOT
                       # used: which provider was attempted first, why it
                       # failed (tool absent / denied / not multimodal),
                       # what extraction path was used instead
  transcript.txt | raw.*
```

`provenance.json` is the receipt — it's what makes the routing rule
falsifiable later instead of an unverifiable claim. If the destination
repository enforces write guards (e.g. direct writes permitted only under
specific subdirectories, everything else requiring a branch/worktree and a
merge), follow that repository's own contribution convention — don't assume
this skill's write path is unrestricted.

## Stage 2 — Research (goal-oriented web research)

Orient research by the **concrete downstream goal**, not "summarize the
source." Fan out, fetch, adversarially verify claims, cite everything. Land
at `research/evaluations/<slug>-<date>.md` in the same research repository
as Stage 1. If the goal is generalizing a single source's claims into a
taxonomy/pattern for this codebase, say so explicitly in the doc so Stage 3
knows what it's grounding against.

## Stage 3 — Independent verification + planning (a different, strong model)

This is the cross-check, not a formality: hand the Stage 1/2 output to a
**model that did not write it**. Two jobs, in order:

1. **Adversarially re-verify** every hard claim against primary sources —
   re-check numbers, quotes, and attributions; correct framing errors in
   place; note what could not be externally verified.
2. **Turn it into an application plan** grounded in the *real* codebase —
   not abstract advice. Survey the actual repo state, cite real files/PRs as
   worked examples, and be explicit about what's still a proposal vs. what's
   already true.

Land at `plans/<date>-<slug>.md`. State the pipeline stage number and next
owner at the top of the file (so a reader mid-pipeline knows where they are).

**This is the human review gate.** Present the plan before moving to Stage
4 — decomposition commits real bead/engineering time, verification doesn't.

## Stage 4 — Decomposition (a third independent model)

A different model again reviews the plan — adversarially, the same as
Stage 3 reviewed the research — before any bead exists. This review is
the point where the pipeline has caught the most real defects in
practice: stale grounding facts, beads that silently bundle several
independently-risky changes, missing dependency edges. Don't skip
straight from plan prose to bead creation without this pass; the plan
author (Stage 3's model) already spent its independence, and a plan
that "looks done" is not the same as a plan an adversarial reader
couldn't break.

**How beads actually get filed depends on size:**

- **Small decomposition (a handful of beads, no phasing needed):** file
  directly via `bd create …`, respecting whatever database the operator has
  configured (e.g. `BEADS_DIR`, or a `-C <path>` override) — don't hardcode a
  specific database path in this skill. Full Wayfinder session overhead
  isn't warranted for 2-3 beads.
- **Large decomposition (needs phased sequencing, dependency graphs,
  multiple work-streams):** drive it through a `wayfinder` session's
  **PLAN** phase (see `wayfinder/PHASES.md` for the nine-phase model —
  CHARTER through RETRO), which files beads through Wayfinder's own
  canonical Beads adapter (`wayfinder/cmd/wayfinder-session/internal/beads`
  in this repo — non-interactive `bd create`, no shell interpolation, empty
  titles rejected). Don't reimplement phased sequencing or a second beads
  filing path in this skill; Wayfinder already owns that mechanic. This is
  the one stage of this pipeline that genuinely overlaps a Wayfinder phase
  (see `docs/design/research-pipeline-wayfinder-integration.md`, once
  merged, for the full incorporate-vs-delegate assessment) — Wayfinder is
  a >1-day, single-project SDLC methodology and this pipeline is not, so
  only this one decomposition mechanic is shared, not a session model.
  Do NOT confuse this with `wayfinder/lib/wayfinder-decompose` — that's an
  unrelated tool that splits one **XL charter** into several **sub-project
  charters** by platform/layer/concern, not for turning a research plan
  into sized beads.

Every bead, however filed, needs:

- a crisp, single-sentence goal
- **testable acceptance criteria** (artifact / exit-code / observable — see
  `docs/skill-verification-criteria.md` for the schema; vague "looks
  correct" criteria are a rejected pattern here too)
- a size (S/M/L) small enough for one Codex `/goal` run
- explicit sequencing/phase if beads depend on each other

## Verification loops, not single-pass gates

Stage 3 and Stage 4 review are not one-shot rubber stamps — treat a
blocking verdict as a normal outcome, not a pipeline failure. When
Stage 4's reviewer finds real, blocking defects (stale grounding facts,
bundled beads, missing dependency edges — not stylistic nitpicks), the
plan routes **back to Stage 3's author to fix**, then Stage 4 re-reviews
the fixed plan. Repeat until the reviewer's verdict is an unconditional
ship (or "ship with inline fixes" the reviewer itself confirms are
non-blocking edits, not open design questions). Decomposing off a
rejected plan just to keep momentum produces beads that fail faster than
no beads at all.

## Stage 5 — Execution (Codex, dark-factory)

Codex agents pick up beads and execute through the normal `safe-pr` /
`safe-merge` gates — no shortcuts because the bead came from this pipeline.
The plan's own proposed gates (if the initiative is about enforcement, e.g.
an eval or lint gate) should apply to the PRs this stage produces, not just
to future work — dogfood the thing you just designed.

## Rules

- **Cross-model, not self-review.** Stages 3 and 4 must each be a model that
  did not author the artifact it's checking. A single model verifying its
  own research is not verification.
- **Provider routing needs a receipt.** See Stage 1 — a silent fallback
  defeats the entire point of naming a provider.
- **Human review before execution commits resources.** Stop and surface the
  Stage 3 plan (and, for large decompositions, the Stage 4 bead list) rather
  than auto-chaining into Stage 5.
- **Ground plans in the real repo, not generic advice.** Stage 3 must cite
  actual files, PRs, or code paths — a plan with no worked example against
  the real codebase is not done.
- **Beads need testable DoD.** No bead ships from Stage 4 with a vague
  acceptance criterion ("looks right", "is complete").
- **A blocking verdict loops back, it doesn't get overridden.** If Stage
  4 finds real defects, fix at Stage 3 and re-review — don't file beads
  off a plan the reviewer rejected because a deadline looms.
- **Don't hardcode operator-specific paths.** Database locations, private
  research-repository names, and similar topology are asked-for or
  configured per session, never assumed by this skill.

## Verification Criteria

The DEAR Auditor (or a self-check, when running solo) checks the following
after this pipeline runs to completion for a given source:

- [ ] The Stage 1 provenance record exists and names the provider actually
      used; if it differs from the provider that best fits the source type,
      the record states why
- [ ] The Stage 2 research doc exists and every hard factual claim carries
      a citation
- [ ] The Stage 3 plan exists, was authored by a model different from the
      one that wrote the research doc, and its verification section names
      at least one correction or explicit confirmation (not silent
      pass-through)
- [ ] Every bead filed from Stage 4 has a non-vague acceptance criterion
      (grep for "looks correct", "is complete", "works well" in bead
      descriptions returns zero matches)
- [ ] The plan was surfaced for human review before any bead moved to
      Stage 5 execution (no PR merged from this pipeline without a review
      gate having occurred first)
- [ ] If Stage 4 review recorded a blocking verdict at any point, a later
      artifact shows the specific corrections applied and a follow-up
      review confirming they're resolved (no bead trail that jumps from
      "blocked" straight to `bd create` with no fix-and-reverify step)

## References

- `evals.json` (co-located) — trigger/behavior eval cases: this skill
  should fire on multi-stage "research → plan → execute" requests and stay
  silent on one-off research questions or bare transcript grabs.
- `docs/skill-verification-criteria.md` — the Form 2 verification-criteria
  schema this skill follows.
- `docs/skill-placement.md` — the placement framework that assigned this
  skill to dear-agent (cross-model verification, human gate before
  execution, sized beads — DEAR process discipline, not personal taste).
- `wayfinder/PHASES.md` and `wayfinder/cmd/wayfinder-session/internal/beads/SPEC.md`
  — the Beads adapter Stage 4 delegates to for large decompositions.
- `docs/design/research-pipeline-wayfinder-integration.md` — the
  incorporate-vs-delegate assessment for this pipeline's one real overlap
  with Wayfinder (decomposition only).
