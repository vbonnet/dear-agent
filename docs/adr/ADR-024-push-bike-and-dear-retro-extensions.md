# ADR-024: Push-Bike Design Constraint and dear-agent's DEAR Retro Extensions

**Status**: Accepted
**Date**: 2026-05-26
**Context**: Per-project amendment that **extends** (does not replace) the
standard DEAR retrospective loop defined in
[/CONTEXT.md § DEAR](../../CONTEXT.md) and used by
[.claude/CLAUDE.md § Agent Delegation Enforcement](../../.claude/CLAUDE.md)
and every file under [docs/retros/](../retros/). Builds on the
[AGENTS.why.md](../../AGENTS.why.md) `.why.md` decision-log pattern —
extended here as the **rip-out-tax tracker** for any deliberately temporary
solution.

Related ADRs (cross-refs updated in this change):

- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
  — the retrospective loop that feeds findings back to the Meta-Orchestrator.
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md) — the
  workflow-engine *code* lifecycle (a separate use of "DEAR"; see
  [/CONTEXT.md § Known Terminology Collisions](../../CONTEXT.md#known-terminology-collisions)).
- [ADR-018: Graceful Exit](ADR-018-graceful-exit-framework-default.md)
  and [ADR-023: Agent Friction Reporting](ADR-023-friction-reporting-and-session-handoff.md)
  — both already cite the DEAR loop and continue to do so unchanged.

> **What this ADR is *not*.** It is **not** a rename of DEAR. The canonical
> four-letter loop **D**efine → **E**xecute → **A**udit → **R**etro is
> unchanged. This ADR adds *project-specific requirements* dear-agent imposes
> on every DEAR retro written in this repo, and names the design constraint
> ("push-bike, not training wheels") that those requirements serve.

---

## Context

The per-task retrospective loop in this repo has been **DEAR** since the
project's founding. It governs how every non-trivial change closes out:
Define exit criteria, Execute the work, Audit the result, Retro the
lessons. The loop is sound, and most retros under `docs/retros/` are
well-served by it.

A meta-retrospective over the most recent retro cluster
(`2026-05-09-ci-red-streak.md`, `2026-05-10-ci-cascade-cleanup.md`,
`2026-05-10-ci-red-and-unguarded-merges.md`, `2026-05-15-backlog-suggestion-system.md`,
`2026-05-17-vroom-doc-drift.md`, `2026-05-21-agm-unified-session-management.md`)
surfaces a recurring failure mode that the four-letter loop, as currently
practised, does **not** catch:

1. **Caps and throttles smuggled in as "fixes".** Several Audit / Retro
   action items added a bound — turn budgets, worker caps, retry ceilings,
   hard-coded denylists, "skip this check in CI" toggles. Each unblocked
   the immediate incident. None scaled. The 2026-05-13 enforcement-rules
   retro (`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`)
   explicitly *rejected* the turn-budget proposal for exactly this reason —
   "training wheels that punish careful work and reward rushed work" — but
   the *pattern* of proposing such caps recurs in other contexts because the
   standard DEAR retro shape does not force the question.
2. **Implementation order is "what we can do now"-first.** The Execute step
   gets specified as "what do we do?" without first asking "what *should*
   the system look like?". The result is repeated retrofits: the cleaner
   shape is discovered on the next retro after the current shape has
   hardened in code and the cost to rip it out is high.
3. **No declared exit conditions for temporary solutions.** Caps that ship
   as "temporary" do not carry a removal trigger. The signal that should
   retire them never arrives because nobody knows what signal to look for.
   The 2026-05-16/05-17 worktree-reaper cluster
   (`memory: dear-agent-worktree-stop-reaper`) shows what this looks like at
   scale: three parallel partial fixes, none removed, each silently widening
   the trust gap.
4. **Diagnose without scale context.** "What broke?" is answered against
   today's load. "What is this same problem at 10× the agents, machines, or
   users?" is not asked, so the fix is sized to today.

The shared thread: dear-agent's DEAR retros are missing a forcing function
that **scale-tests the proposed fix before it ships**, and a forcing
function that makes temporary fixes carry their own removal instructions.

These are project-specific concerns. The DEAR framework itself is fine; this
repo's *use* of it needs sharpening. Other repos that adopt DEAR may layer
their own additions for their own concerns — this ADR documents one set of
additions and the mechanism by which any repo can declare its own.

---

## Decision

Five amendments, taken as a bundle. **All are dear-agent project-specific
additions to the standard DEAR retro — DEAR itself is unchanged.**

### D1. dear-agent's DEAR retros MUST include four extra sections

Every DEAR retro written in this repo MUST include, in addition to the
standard Define / Execute / Audit / Retro structure, the following:

| Section name (where it lives in the retro)               | What it carries                                                                                                                                                                                                                                                                                                                                                            |
|---------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Scaling model** (subsection inside Define)             | Current scale, target scale (12-month horizon), scaling model (linear / sub-linear / super-linear in what dimension), binding constraint at target. Unknowns are filled with "unknown — see follow-up to measure" rather than removed.                                                                                                                                       |
| **Ideal-first solution** (two-pass write order in Execute) | Write the ideal scalable solution **first**, with no reference to current code or this-week constraints. Then write the **path-compatible minimum** — the smallest change we can ship now that is on the path to the ideal. If the minimum has to be ripped out to reach the ideal, reject it and restart at the ideal.                                                       |
| **Scale-test** (subsection inside Audit)                 | Score every proposed fix against three axes — 10× agents, 10× machines, 10× users — as **scales** / **neutral** / **caps**. A fix scoring `caps` on any axis is permitted only if it carries a rip-out tax `.why.md` (D2).                                                                                                                                                  |
| **Rip-out tax declarations** (cross-link inside Scale-test) | For every `caps`-scored fix, a co-located `.why.md` file. Same pattern as the existing AGENTS.why.md decision logs — see D2.                                                                                                                                                                                                                                                |

These sections are MANDATORY for new retros in this repo and are baked
into the template at [docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md).

Historical retros under `docs/retros/` are **not retroactively amended**
(append-only audit trail per `docs/alignment/VALUES.md`).

### D2. Any cap ships with a co-located `.why.md` (rip-out tax)

Any code change that scores **caps** on any Scale-test axis (throttle, cap,
denylist, retry ceiling, hard-coded ID, feature flag set to a non-default
value, "skip in CI" toggle, etc.) MUST ship with a co-located `.why.md`
file. The file states:

```markdown
# <thing>.why.md — temporary cap

**Type:** cap (rip-out tax)
**Date:** YYYY-MM-DD
**Why this is here:** <the load it relieves, the regression it prevents>
**What it costs to remove:**
  - Code touched: <files / packages / call sites>
  - Migration shape: <what has to change to delete this cleanly>
**Removal trigger:** <the observable signal that says "now safe to remove">
**Related retro:** <link to the DEAR retro that introduced it>
```

This **extends** the `.why.md` pattern already established by
[AGENTS.why.md](../../AGENTS.why.md): the pattern already documents
deliberate decisions co-located with the code they explain. Caps are
deliberate decisions deserving co-located rationale — same pattern, broader
scope. We reuse the existing mechanism rather than invent a parallel
ledger.

A cap without a `.why.md` is a code-review fail. A CI grep gate that
enforces it mechanically is listed under "Open questions" — out of scope
for this ADR.

### D3. The named principle: push-bike, not training wheels

The design constraint that all four extra sections serve:

> A **push-bike** is the simplest shape of the *eventual* bike — frame,
> wheels, no pedals — that a child rides on the path to a real bike.
> Everything they learn (balance, steering, momentum) transfers. Nothing
> has to be ripped out when pedals arrive.
>
> **Training wheels** are bolted-on caps that prevent the child from
> learning the thing that actually matters (balance) and must be removed
> before they can ride for real. Every training-wheel solution buys time at
> the cost of teaching the wrong reflex.
>
> **Design constraint:** prefer push-bike solutions. When a training-wheel
> solution is the only option this week, declare it as one (rip-out tax,
> D2) and schedule its removal.

This phrase is the named principle that agents and humans cite when
rejecting a proposed fix. It appears verbatim in:

- [/CONTEXT.md § Push-bike, not training wheels — design constraint for DEAR retros (dear-agent)](../../CONTEXT.md#push-bike-not-training-wheels--design-constraint-for-dear-retros-dear-agent)
- [.claude/CLAUDE.md § Push-bike, not training wheels](../../.claude/CLAUDE.md)
- [docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md)
- [AGENTS.why.md § Why the `.why.md` pattern is also the rip-out-tax tracker](../../AGENTS.why.md)

Citing "push-bike, not training wheels" in a review or retro is a
legitimate veto — it forces the proposer to either reframe the fix or
attach a rip-out tax.

### D4. Per-project extension mechanism

DEAR itself is a generic, four-letter loop. Different projects have
different concerns, and a one-size-fits-all template would either be too
generic to be useful or too specific to suit other repos.

The mechanism: **a project that adopts DEAR may declare additional retro
requirements in three project-scope artifacts**, each playing a distinct
role (same instruction / configuration / enforcement separation that
[AGENTS.why.md](../../AGENTS.why.md) describes for output routing):

| Tier               | Mechanism                                | Role                                              |
|--------------------|------------------------------------------|---------------------------------------------------|
| **Instruction**    | `.claude/CLAUDE.md` in the repo           | Tells agents what the additional requirements are |
| **Rationale**      | A project ADR (this file, for dear-agent) | Records *why* those requirements exist            |
| **Template**       | `docs/retros/_TEMPLATE.md` (or repo equivalent) | Provides the concrete shape new retros copy from |

A retro is a "DEAR retro under repo X's requirements" if it follows DEAR's
four phases and the repo's declared additions. There is no global registry
of project additions — each repo's `.claude/CLAUDE.md` is the discovery
point for an agent that lands there.

This is by design: DEAR stays universal and easy to teach; project-specific
sharpening lives in project-scope files where it can be added or removed
without coordinating across repos.

### D5. dear-agent's additions become its own DEAR retro template

[docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md) is **dear-agent's** DEAR
retro template. It is **not** a generic DEAR template — it carries the four
project-specific additions from D1. A new retro in this repo copies the
template; a retro in a different repo does not.

The template's mandatory sections are:

```
# DEAR Retro: <title>
## Define
  ### Scaling model         ← dear-agent addition (D1)
## Execute
  ### Ideal-first solution  ← dear-agent addition (D1)
  ### Path-compatible minimum
## Audit
  ### Scale-test            ← dear-agent addition (D1)
  ### Rip-out tax declarations ← dear-agent addition (D1 / D2)
## Retro
  ### Action items
```

The four phase headings are DEAR-canonical; the four subsection headings
under them are dear-agent's project additions.

---

## What changes (in this PR)

| File                                             | Change                                                                                          |
|--------------------------------------------------|-------------------------------------------------------------------------------------------------|
| `docs/adr/ADR-024-...md` (this file)             | New                                                                                              |
| `/CONTEXT.md`                                    | New § "Push-bike, not training wheels — design constraint for DEAR retros (dear-agent)" and § "dear-agent's additions to the standard DEAR retro"; DEAR section itself unchanged in its canonical letters |
| `.claude/CLAUDE.md`                              | New § "Push-bike, not training wheels (MANDATORY design constraint)"; DEAR-loop bullet adds a pointer to the template and the additional requirements |
| `AGENTS.why.md`                                  | New section: `.why.md` is also the rip-out-tax tracker for caps                                  |
| `docs/retros/_TEMPLATE.md`                       | New: dear-agent's DEAR retro template with the four mandatory project additions                  |

## What does **not** change

- **The DEAR framework itself.** Define → Execute → Audit → Retro is the
  canonical four-letter loop; the letters and their meanings are
  unchanged. dear-agent layers requirements *inside* those phases without
  altering the loop.
- **The workflow-engine `OnDefine` / `OnEnforce` / `OnAudit` / `OnResolve`
  callbacks (ADR-010 / ADR-011).** That is a separate use of "DEAR" —
  Define → Enforce → Audit → Resolve & Refine — and is unrelated to the
  process retro loop. The collision was already documented in
  [/CONTEXT.md § Known Terminology Collisions](../../CONTEXT.md#known-terminology-collisions).
- **Historical retros under `docs/retros/`.** Not rewritten — append-only.
- **`pkg/workflow` schema, runner, hooks.** No code changes ship with this
  ADR.
- **Other repos' DEAR practice.** The additions in D1 are dear-agent-scope.
  Per D4, each repo declares its own.

---

## Consequences

### Positive

- **Retros stop smuggling in caps.** The Scale-test subsection makes "this
  is a cap" visible at the moment of proposal, not three retros later.
- **The cleaner shape is discovered first.** Ideal-first → scoped-down
  avoids the retrofit penalty: every shipped minimum is on the path to the
  ideal.
- **Temporary solutions carry their own removal trigger.** The `.why.md`
  rip-out tax means "schedule removal" is not a TODO that decays — the file
  lives next to the code that has to change to remove it.
- **DEAR stays clean and portable.** The four-letter loop continues to
  mean exactly what it has always meant; project sharpening lives in
  project-scope files. Other repos that adopt DEAR are not forced to take
  on dear-agent's additions.
- **A named principle to cite.** "Push-bike, not training wheels" gives
  reviewers and supervisors a one-phrase veto that everyone parses the
  same way.
- **`.why.md` pattern broadened, not duplicated.** Reuses an existing
  mechanism instead of inventing a parallel ledger.

### Negative / costs

- **dear-agent retros get longer.** Define grows by a scale block, Execute
  grows by an ideal-first description, Audit grows by Scale-test scoring.
  Accepted; the cost of a longer retro is small compared to the cost of a
  cap shipped without one.
- **The additions are repo-specific. New contributors must read
  `.claude/CLAUDE.md` and the template to discover them.** Mitigated by
  the template being the obvious starting point for any new retro.
- **`.why.md` files accumulate.** A future audit (deferred) should grep
  for expired removal triggers.

### Neutral

- **No CI gate ships with this ADR.** The "cap without `.why.md`" check
  is an open question (below).
- **The workflow-engine `OnAudit` etc. retain the "DEAR" naming.** The
  collision is unchanged.

---

## Alternatives considered

| Alternative                                              | Why rejected                                                                                                                                                                |
|----------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Rename DEAR → DEASR (insert "Scale-test" as a fifth letter).** | Considered and rejected by Valentin: "much cleaner" to keep DEAR. The forcing function is what matters, not the acronym. We add a *section* inside Audit, not a fifth phase. |
| **Mandate the additions globally (in DEAR itself).**     | Other projects may have other forcing-function needs. Universalising dear-agent's additions presumes our concerns are everyone's. The per-project mechanism (D4) avoids that. |
| **Make scale-test optional ("when relevant").**          | Optional steps are skipped. Mandatory + cheap (3 rows in a table) is the right shape.                                                                                       |
| **Ban caps outright.**                                   | Sometimes a cap is the right call (e.g. classifier blocks on hard-to-reverse actions). The rip-out tax is the correct response, not a ban.                                  |
| **Use a separate `caps.md` ledger instead of `.why.md`.** | Splits the substrate — caps live near code, ledgers live far away, drift accumulates. The `.why.md` pattern already exists and already lives next to the thing it explains. |
| **Ship just the philosophy section, defer template.**    | The template is what makes the requirements automatic; without it the "additions" decay back to folklore.                                                                   |

---

## Open questions

Deferred to follow-up ADRs / point releases:

1. **CI gate for `.why.md` on caps.** A lightweight check that fails CI
   when a PR introduces a new throttle / cap / denylist without a
   co-located `.why.md`. Needs a definition of "cap" the grep can
   recognise. Plausible first cut: a `// rip-out-tax:` line comment that
   names the `.why.md`.
2. **Expired-trigger audit.** A periodic check that walks `.why.md`
   files, evaluates their removal triggers against current metrics, and
   files a finding when a cap should be retired. Belongs under ADR-011's
   audit subsystem.
3. **Scale-test scoring stored as data, not prose.** Today the Scale-test
   table is markdown. A follow-up could give it a YAML twin so the
   substrate can query "every fix that ever scored *caps* on Users" —
   but the markdown table is the right shape for v1.
4. **A second repo adopts the per-project DEAR-extension mechanism.**
   Until another repo declares its own additions via D4, the mechanism is
   theoretical. The first cross-repo test (e.g. brain-v2 or
   engram-research) is what proves the seam.

---

## Status

This ADR is **Accepted**. It commits the project to D1–D5 above for new
process retrospectives in this repo. Existing retros stay in their current
form. DEAR itself — the four-letter loop — is unchanged by this ADR.

Acceptance does NOT pre-authorise:

- Renaming the workflow-engine `OnAudit` / `OnResolve` callbacks (the
  separate "DEAR" collision in CONTEXT.md; a different ADR owns that
  rename).
- Renaming the `dear-agent` project or any `cmd/dear-agent-*` binary.
- A CI gate that enforces the rip-out tax (open question 1).
- Rewriting historical retros (append-only).
- Imposing dear-agent's additions on other repos.

---

## References

- [/CONTEXT.md § DEAR](../../CONTEXT.md)
- [/CONTEXT.md § Push-bike, not training wheels — design constraint for DEAR retros (dear-agent)](../../CONTEXT.md#push-bike-not-training-wheels--design-constraint-for-dear-retros-dear-agent)
- [AGENTS.why.md § rip-out-tax tracker](../../AGENTS.why.md)
- [.claude/CLAUDE.md § Push-bike, not training wheels](../../.claude/CLAUDE.md)
- [docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md)
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md) — workflow-engine code lifecycle (separate use of "DEAR"; not renamed)
- [ADR-018: Graceful Exit](ADR-018-graceful-exit-framework-default.md) — same two-tier (instruction + configuration) shape this ADR borrows
- 2026-05-13 enforcement-rules retro
  (`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`) —
  the turn-budget rejection that named the "training wheels that punish
  careful work" failure mode this ADR generalises.
