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
 │           │   │  cite, verify)│   │ grounded plan       │   │ beads, file through │   │ via      │
 │           │   │               │   │                     │   │ the repository's    │   │ repo     │
 │           │   │               │   │                     │   │ canonical Beads CLI │   │ gates    │
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
5. **Execute** — beads move through the target repository's documented
   delivery interface (`safe-*` only where provided).

## Not every task needs all five stages

| You have…                                            | Start at                              |
|-------------------------------------------------------|----------------------------------------|
| A source (video/article) and nothing else            | Stage 1 (ingest)                       |
| Raw source already saved, need it understood          | Stage 2 (research)                     |
| A research doc you're not sure is trustworthy         | Stage 3 (independent verify + plan)    |
| A verified plan, need it actionable                   | Stage 4 (decompose into beads)         |
| Sized beads with a DoD, ready to build                | Stage 5 (Codex / repository gates)     |
| Just "what does this video say"                       | Stop after Stage 1-2 — don't chain     |

Chain forward only as far as the concrete downstream goal requires. A
one-off curiosity question stops at Stage 2. Only chain to Stage 4/5 when
the destination is genuinely a codebase change.

## Stage 1 — Ingest: route to the right provider, keep a receipt

**Rule: name the provider that's actually right for the source type — never
silently let whichever harness is running absorb the task.** YouTube/video →
a video-capable provider first (e.g. Gemini). If the running harness lacks
that provider capability, use an available fallback and record both the
capability gap and the fallback. If the provider call is denied by the user or
the permission system, stop and defer or escalate that stage; denial is not
authorization to bypass the decision with another provider. A headless run
may fall back only when the named provider is genuinely unavailable, not when
an approval request was rejected.

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
                       # was unavailable (tool absent / not multimodal),
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
Presentation alone is not approval: record an explicit human approval receipt
(reviewer identity, timestamp, durable comment/artifact reference, and the
approved plan revision's commit SHA or content digest) before the first bead is
created. If no approval arrives, pause at Stage 3. Approval applies only to the
identified plan revision; later substantive corrections require a new receipt.

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

Before filing anything, verify that the Stage 3 human approval receipt names
the exact plan revision under review and predates the first prospective bead.
The independent Stage 4 model review supplements that human gate; it does not
replace it.

**File every decomposition through the target repository's canonical Beads
interface.** Check its `AGENTS.md`/equivalent first — e.g. dear-agent itself
requires the explicit form
`bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>`
for every invocation; use that exact form here. For a repo with no documented
policy, respect the operator's configured database through a supported
`--db <path>` or `-C <path>` database-selection override on the canonical
invocation instead of hardcoding a path. That fallback is for the *absence*
of repo policy, not license to skip required flags. The portable interface is
self-contained here: inspect with `ready`, `list --status=open`, and
`show <id>`; create durable work with
`create "<title>" --description="<goal and acceptance>" --type=<type>
--priority=<0-4>`; add dependency edges with `dep add <dependent>
<dependency>`; and use non-interactive `update`/`close` flags only when the
target repository's policy authorizes those state changes. Prefer `--json`
whenever output is parsed, never use interactive edit, and never treat a local
plan or scratch file as a substitute for the shared Beads record.

For a small decomposition, create the handful of beads directly. For a large
decomposition, still create the beads directly, then encode its phases,
dependency edges, and parallel work-streams with the canonical Beads
subcommands. If a request calls the result "Wayfinder beads," correct that
terminology and continue this Stage 4 flow through canonical Beads; the
misnomer does not make the request out of scope. Do not route bead creation
to Wayfinder's PLAN phase:
`wayfinder-session start_phase` creates a single session-level bead during
SETUP or BUILD, and PLAN cannot start until SPEC; it is not a research-plan
decomposition API. After the reviewed bead graph exists, inspect the target
repository's delivery policy. If its `safe-pr` (or equivalent) gate requires
an in-progress Wayfinder session, start that session before any Stage 5
execution; if Wayfinder is optional, the operator may still choose it for a
multi-phase initiative. In either case, Wayfinder does not replace or own
Stage 4 filing.

Every bead, however filed, needs:

- a crisp, single-sentence goal
- **testable acceptance criteria** (artifact / exit-code / observable — see
  `../../docs/skill-verification-criteria.md` for the packaged schema; vague "looks
  correct" criteria are a rejected pattern here too)
- a size (S/M/L) small enough for one Codex `/goal` run
- explicit sequencing/phase if beads depend on each other

## Verification loops, not single-pass gates

Stage 3 and Stage 4 review are not one-shot rubber stamps — treat a
blocking verdict as a normal outcome, not a pipeline failure. When
Stage 4's reviewer finds real, blocking defects (stale grounding facts,
bundled beads, missing dependency edges — not stylistic nitpicks), the
plan routes **back to Stage 3's author to fix**, then Stage 4 re-reviews
the fixed plan. Because those corrections change what would be decomposed,
present the corrected revision to the human and record a new approval receipt
bound to that exact commit SHA or content digest before Stage 4 re-review or
bead filing. Repeat until the reviewer has assessed the human-approved
revision and its verdict is an unconditional
ship (or "ship with inline fixes" the reviewer itself confirms are
non-blocking edits, not open design questions). Decomposing off a
rejected plan just to keep momentum produces beads that fail faster than
no beads at all.

## Stage 5 — Execution (Codex, dark-factory)

Codex agents pick up beads and execute through the target repository's
documented delivery interface—without shortcuts because the bead came from
this pipeline. Use `safe-pr` / `safe-merge` only when that repository actually
provides and requires those wrappers; otherwise use its documented equivalent
commit, push, review, and merge-readiness gates. Before dispatching, record the
exact approved set of in-scope bead IDs, then satisfy every target-repository
prerequisite for that interface, including establishing an in-progress
Wayfinder session when its delivery policy mandates one.
The plan's own proposed gates (if the initiative is about enforcement, e.g.
an eval or lint gate) should apply to the PRs this stage produces, not just
to future work — dogfood the thing you just designed.

## Rules

- **Cross-model, not self-review.** Stage 3 must differ from the Stage 2
  research author; Stage 4 must differ from both the Stage 2 and Stage 3
  models. A model never verifies an artifact derived from its own pipeline work.
- **Provider routing needs a receipt.** See Stage 1 — a silent fallback
  defeats the entire point of naming a provider.
- **Human approval before decomposition commits resources.** Stop after
  surfacing the Stage 3 plan until an explicit approval receipt is recorded.
  Bind the receipt to the exact plan commit SHA or content digest, and obtain a
  fresh receipt after any substantive correction to a rejected plan.
  Do not create the Stage 4 bead graph merely because the plan was presented;
  for large decompositions, surface the reviewed bead list again before Stage
  5 execution.
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
- [ ] The Stage 4 review names a model distinct from both the Stage 2 research
      author and the Stage 3 planner
- [ ] Every bead filed from Stage 4 has a non-vague acceptance criterion
      (grep for "looks correct", "is complete", "works well" in bead
      descriptions returns zero matches)
- [ ] A durable human approval receipt (reviewer, timestamp, reference)
      names the exact plan revision accepted by Stage 4 and predates the
      `created_at` time of the first Stage 4 bead; merely surfacing the plan,
      approving an earlier revision, or obtaining approval before Stage 5 does
      not pass
- [ ] If Stage 4 review recorded a blocking verdict at any point, a later
      artifact shows the specific corrections applied, a fresh human approval
      receipt bound to the corrected revision, and a follow-up review
      confirming they're resolved (no bead trail that jumps from "blocked"
      straight to bead filing with no fix, reapproval, and reverify step)
- [ ] If the requested scope includes Stage 5, every bead in the recorded
      approved execution set is dispatched and satisfies the target repository's
      documented merge, deploy, and real-surface definition of done. No in-scope
      bead remains undispatched. Open PRs, incomplete work, or unresolved proof
      are recorded as a partial Stage 4 stop with status and blockers, not
      pipeline completion

## References

- `evals.json` (co-located) — trigger/behavior eval cases: this skill
  should fire on multi-stage "research → plan → execute" requests and stay
  silent on one-off research questions or bare transcript grabs.
- `../../docs/skill-verification-criteria.md` — the packaged Form 2 verification-criteria
  schema this skill follows.
- `docs/skill-placement.md` — the placement framework that assigned this
  skill to dear-agent (cross-model verification, human gate before
  execution, sized beads — DEAR process discipline, not personal taste).
- Stage 4 above contains the portable direct Beads interface shipped with this
  standalone plugin. A target repository's own `AGENTS.md` or Beads skill may
  tighten that interface; Wayfinder may orchestrate a later SDLC session but
  does not own bead filing.
