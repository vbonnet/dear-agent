# ADR-024: DEASR Retrospective Loop and Push-Bike-First Design

**Status**: Accepted
**Date**: 2026-05-26
**Context**: Process amendment to the per-task retrospective loop defined in
[/CONTEXT.md § DEAR](../../CONTEXT.md) and used by
[.claude/CLAUDE.md § Agent Delegation Enforcement](../../.claude/CLAUDE.md)
and every file under [docs/retros/](../retros/). Builds on the two-tier
(instruction + configuration) rationale in
[AGENTS.why.md](../../AGENTS.why.md) — specifically the **`.why.md` decision-log
pattern**, which this ADR repurposes as the rip-out-tax tracker for any
deliberately temporary solution.

Related ADRs (cross-refs updated in this change):

- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
  — the retrospective loop that feeds findings back to the Meta-Orchestrator.
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md) — the
  workflow-engine *code* lifecycle. **Not renamed by this ADR.** Disambiguation
  banner updated to name the process loop "DEASR" explicitly.
- [ADR-018: Graceful Exit as a Framework Default](ADR-018-graceful-exit-framework-default.md)
  and [ADR-023: Agent Friction Reporting](ADR-023-friction-reporting-and-session-handoff.md)
  — both already cite the process retro loop; cross-refs renamed to DEASR.

---

## Context

The per-task retrospective loop has been **DEAR** (Define → Execute → Audit →
Retro) since the founding of this repo. It governs how every non-trivial change
closes out: state the exit criteria, do the work, verify, capture lessons. The
loop is sound, and most retros under `docs/retros/` are well-served by it.

A meta-retrospective over the most recent retro cluster
(`2026-05-09-ci-red-streak.md`, `2026-05-10-ci-cascade-cleanup.md`,
`2026-05-10-ci-red-and-unguarded-merges.md`, `2026-05-15-backlog-suggestion-system.md`,
`2026-05-17-vroom-doc-drift.md`, `2026-05-21-agm-unified-session-management.md`)
surfaces a recurring failure mode that the four-letter loop does **not** catch:

1. **Caps and throttles smuggled in as "fixes".** Several Act items added a
   bound — turn budgets, worker caps, retry ceilings, hard-coded denylists,
   "skip this check in CI" toggles. Each unblocked the immediate incident.
   None scaled. The 2026-05-13 enforcement-rules retro
   (`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`)
   explicitly *rejected* the turn-budget proposal for exactly this reason —
   "training wheels that punish careful work and reward rushed work" — but
   the *pattern* of proposing such caps recurs in other contexts because the
   retro template does not force the question.
2. **Implementation order is "what we can do now"-first.** The Act step asks
   "what do we do?" without first asking "what *should* the system look
   like?". The result is repeated retrofits: the cleaner shape is discovered
   on the next retro after the current shape has hardened in code and the
   cost to rip it out is high.
3. **No declared exit conditions for temporary solutions.** Caps that ship as
   "temporary" do not carry a removal trigger. The signal that should retire
   them never arrives because nobody knows what signal to look for. The
   2026-05-16/05-17 worktree-reaper cluster
   (`memory: dear-agent-worktree-stop-reaper`) shows what this looks like at
   scale: three parallel partial fixes, none removed, each silently widening
   the trust gap.
4. **Diagnose without scale context.** "What broke?" is answered against
   today's load. "What is this same problem at 10× the agents, machines, or
   users?" is not asked, so the fix is sized to today.

The shared thread: the loop is missing a forcing function that **scale-tests
the proposed fix before it ships**, and a forcing function that makes
temporary fixes carry their own removal instructions.

---

## Decision

Four amendments to the process retrospective loop, taken as a bundle.

### D1. Rename DEAR → DEASR; add a Scale-Test phase between Evaluate and Act

The process loop becomes **DEASR**:

| Letter | Phase           | Meaning                                                                                                  |
|--------|-----------------|----------------------------------------------------------------------------------------------------------|
| **D**  | **Diagnose**    | State the task / incident, **current scale**, **target scale**, and the **scaling model** that connects them. |
| **E**  | **Evaluate**    | Describe the **ideal scalable solution first**, then scope down to the minimum on the path to it.        |
| **A**  | **Scale-test (Assay)** | Score every proposed fix against 10× agents / 10× machines / 10× users: **scales**, **neutral**, or **caps**. |
| **S**  | ~~Scale-test~~  | *(letter assigned to Scale-test above — kept here as a navigation aid; not a separate phase.)*           |
| **R**  | **Review**      | Capture what was learned. Findings flow to the backlog/roadmap via the Meta-Orchestrator.                |

> **Letter mapping note.** The mnemonic is **D-E-A-S-R** = Diagnose, Evaluate,
> sc**A**le-test (Assay), **S**cope-down→Act, **R**eview. The previous
> letters do not survive 1:1; the alignment is intentional so the rename is
> visibly *not* a sticker over DEAR. We accept the slightly forced fit because
> the *forcing functions* the new letters represent (scale awareness + ideal-first
> + scale scoring + reversible-default) are what the practice was missing,
> not the prior word choices.

Old (DEAR) → new (DEASR) mapping, for retros being updated:

| Old phase  | New phase(s)                                            |
|------------|----------------------------------------------------------|
| Define     | **Diagnose** (adds the scale context block)              |
| Execute    | **Evaluate** (now explicit about ideal-first) + **Act** (now the scoped-down build) |
| Audit      | **Scale-test** (new forcing question) + Act-verification |
| Retro      | **Review** (unchanged in spirit; renamed for the R slot) |

Historical retros under `docs/retros/` are **not retroactively renamed**
(append-only audit trail per `docs/alignment/VALUES.md`). New retros use the
DEASR template at [docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md).

### D2. Diagnose carries explicit architectural / scale context

Every Diagnose section MUST include a "Scaling model" block:

```markdown
### Scaling model

- **Current scale:** <today's numbers — agents, sessions, repos, requests/min>
- **Target scale:** <where we credibly expect to be in 12 months>
- **Scaling model:** <linear / sub-linear / super-linear in what dimension>
- **Binding constraint at target:** <what runs out first — tokens, FDs, locks, humans>
```

A retro that cannot fill this block has no basis to declare a solution "good
enough" — it is sizing a fix to a load nobody has measured. In that case,
filling the block (even with "unknown — see follow-up to measure") is itself
an action item.

### D3. Evaluate describes the ideal first, then scopes down

The Evaluate section MUST be written in two passes, in this order:

1. **Ideal scalable solution.** Describe the shape the system would take if we
   were rebuilding it today with the target scale in mind. No reference to
   what currently exists. No reference to what we can ship this week.
2. **Path-compatible minimum.** Describe the smallest change we can ship now
   that is **on the path to (1)** — i.e. nothing in it has to be ripped out
   to get there. If the minimum has to be ripped out, it is rejected and
   restart at (1).

The order matters. Writing (2) first anchors on the current shape and the
ideal is reverse-engineered to justify it; that produces the failure mode this
ADR exists to correct.

### D4. Scale-test (Assay): every fix scored scales / neutral / caps

Each proposed Act item is scored against three axes:

| Axis        | Question                                                            |
|-------------|---------------------------------------------------------------------|
| **Agents**  | If 10× agents hit this code path simultaneously, what happens?      |
| **Machines**| If 10× machines run this work in parallel, what happens?            |
| **Users**   | If 10× users depend on this behaviour, what happens?                |

Each axis gets one label:

- **scales** — the fix gets *better* (or stays linear) at 10×.
- **neutral** — the fix is unaffected; no improvement, no regression.
- **caps** — the fix actively limits, throttles, denylists, or hard-codes.

A fix scoring **caps** on any axis is **not banned** — sometimes a cap is the
right call — but it MUST then carry a rip-out tax declaration (D5). A fix
scoring **scales** on every axis ships without further ceremony.

The scoring lives in a "Scale-test" subsection of the retro, before "Act":

```markdown
### Scale-test

| Proposed fix         | Agents  | Machines | Users   | Rip-out tax (if caps) |
|----------------------|---------|----------|---------|------------------------|
| `OnAudit` rate-limit | caps    | neutral  | neutral | `pkg/audit/limit.why.md` |
| Audit-event index    | scales  | scales   | scales  | —                      |
```

### D5. Rip-out tax: every cap carries a `.why.md`

Any proposed solution that scores **caps** on any axis MUST declare, in the
same change, a `.why.md` file co-located with the code it caps. The file
states:

```markdown
# <thing>.why.md — temporary cap

**Type:** cap (rip-out tax)
**Date:** <YYYY-MM-DD>
**Why this is here:** <the load it relieves, the regression it prevents>
**What it costs to remove:**
  - Code touched: <files / packages / call sites>
  - Migration shape: <what has to change to delete this cleanly>
**Removal trigger:** <the observable signal that says "now safe to remove">
**Related retro:** <link to the DEASR retro that introduced it>
```

This is the same `.why.md` pattern already established by
[AGENTS.why.md](../../AGENTS.why.md) for co-located decision logs. This ADR
extends it: any temporary solution (cap, throttle, denylist, retry ceiling,
hard-coded ID, feature flag set to a non-default value, "skip in CI" toggle)
counts as a deliberate decision deserving a `.why.md` next to the code.

The forcing function: a cap without a `.why.md` is a code-review fail. CI gets
a lightweight grep check in a follow-up (out of scope for this ADR — listed
under "Open questions").

### D6. Push-bike, not training wheels — engrained as a design constraint

The principle named:

> A **push-bike** is the simplest shape of the *eventual* bike — frame,
> wheels, no pedals — that a child rides on the path to a real bike. Everything
> they learn (balance, steering, momentum) transfers. Nothing has to be ripped
> out when pedals arrive.
>
> **Training wheels** are bolted-on caps that prevent the child from learning
> the thing that actually matters (balance) and must be removed before they
> can ride for real. Every training-wheel solution buys time at the cost of
> teaching the wrong reflex.
>
> **Design constraint:** prefer push-bike solutions. When a training-wheel
> solution is the only option this week, declare it as one (rip-out tax, D5)
> and schedule its removal.

This phrase is the named principle agents and humans cite when rejecting a
proposed fix. It appears verbatim in:

- [/CONTEXT.md § DEASR](../../CONTEXT.md#deasr--diagnose--evaluate--scale-test--act--review)
- [.claude/CLAUDE.md § DEASR loop](../../.claude/CLAUDE.md)
- [docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md)
- [AGENTS.why.md § Why the `.why.md` pattern is also the rip-out-tax tracker](../../AGENTS.why.md)

Citing "push-bike, not training wheels" in a review or retro is a legitimate
veto — it forces the proposer to either reframe the fix or attach a rip-out
tax.

---

## What changes (in this PR)

| File                                             | Change                                                                 |
|--------------------------------------------------|------------------------------------------------------------------------|
| `docs/adr/ADR-024-...md` (this file)             | New                                                                    |
| `/CONTEXT.md`                                    | DEAR → DEASR throughout; new § DEASR with full phase definitions; updated diagram, framework table, glossary, collisions register |
| `.claude/CLAUDE.md`                              | "DEAR loop" callout becomes "DEASR loop"; push-bike rule added         |
| `AGENTS.why.md`                                  | New section: `.why.md` is also the rip-out-tax tracker for caps        |
| `docs/retros/_TEMPLATE.md`                       | New: DEASR retro template with Diagnose/Evaluate/Scale-test/Act/Review |
| `docs/adr/ADR-002-vroom-execution-architecture.md` | DEAR → DEASR in two prose references                                |
| `docs/adr/ADR-011-dear-audit-subsystem.md`       | Disambiguation banner names the process loop "DEASR" explicitly        |
| `docs/adr/ADR-018-graceful-exit-framework-default.md` | Disambiguation banner names the process loop "DEASR" explicitly   |
| `docs/adr/ADR-023-friction-reporting-and-session-handoff.md` | DEAR → DEASR where the reference is to the process loop    |

## What does **not** change

- **The workflow-engine `OnDefine` / `OnEnforce` / `OnAudit` / `OnResolve`
  callbacks (ADR-010 / ADR-011) keep their names.** That is a *code lifecycle*,
  not a process loop, and renaming exported callback names is a hard-to-reverse
  API change deserving its own ADR. The collision is still flagged in
  [/CONTEXT.md § Known Terminology Collisions](../../CONTEXT.md#known-terminology-collisions),
  now with crisper letters (DEASR vs. workflow-engine DEAR) — the two names no
  longer expand to the same string.
- **The `dear-agent` repo / project name.** The repo predates the loop and is
  not derived from it.
- **Historical retros under `docs/retros/`** are not rewritten. New retros use
  the DEASR template.
- **`pkg/workflow` schema, runner, hooks.** No code changes ship with this ADR.

---

## Consequences

### Positive

- **Retros stop smuggling in caps.** The Scale-test phase makes "this is a cap"
  visible at the moment of proposal, not three retros later.
- **The cleaner shape is discovered first.** Ideal-first → scoped-down avoids
  the retrofit penalty: every shipped minimum is on the path to the ideal.
- **Temporary solutions carry their own removal trigger.** The `.why.md`
  rip-out tax means "schedule removal" is not a TODO that decays — the file
  lives next to the code that has to change to remove it.
- **A named principle to cite.** "Push-bike, not training wheels" gives
  reviewers and supervisors a one-phrase veto that everyone parses the same
  way.
- **`.why.md` pattern broadened, not duplicated.** Reuses an existing mechanism
  instead of inventing a parallel ledger.

### Negative / costs

- **Retros get longer.** Diagnose grows by a scale block, Evaluate grows by an
  ideal-first description, Scale-test is new. We accept this; the cost of a
  longer retro is small compared to the cost of a cap shipped without one.
- **The DEAR → DEASR rename costs cross-reference churn.** Mitigated by doing
  the sweep in this PR (eight files) rather than letting it drift.
- **The letter mapping is slightly forced** (sc**A**le-test → A, **S**cope-down
  → S as the same phase). Accepted; the forcing functions matter more than the
  acronym aesthetic.
- **`.why.md` files accumulate.** A future audit (deferred) should grep for
  expired removal triggers.

### Neutral

- **Workflow-engine `OnAudit` etc. are unaffected.** The collision is now
  *visible* but not *resolved* — that resolution is still pending an ADR that
  owns the code rename.
- **No CI gate ships with this ADR.** The "cap without `.why.md`" check is an
  open question (below).

---

## Alternatives considered

| Alternative                                         | Why rejected                                                                                                                                                                |
|-----------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Keep DEAR; just add a "Scale-test" checkbox.**    | The forcing function only works if it is a named phase, not a sub-bullet. Sub-bullets get skipped under time pressure; phase headings do not.                              |
| **Rename DEAR → DEAR'** (prime), keeping letters.    | Confusing in conversation; provides no mnemonic separation from the workflow-engine DEAR. The collision would persist.                                                     |
| **Replace DEAR entirely with a new framework name.** | Continuity matters — `docs/retros/` has a year of history under "DEAR Retro" titles. DEASR keeps the family resemblance while signalling the change.                       |
| **Ban caps outright.**                              | Sometimes a cap is the right call (e.g. classifier blocks on hard-to-reverse actions). The rip-out tax is the correct response, not a ban.                                  |
| **Use a separate `caps.md` ledger instead of `.why.md`.** | Splits the substrate — caps live near code, ledgers live far away, drift accumulates. The `.why.md` pattern already exists and already lives next to the thing it explains. |
| **Make scale-test optional ("when relevant").**     | Optional steps are skipped. Mandatory + cheap (3 rows in a table) is the right shape.                                                                                       |

---

## Open questions

Deferred to follow-up ADRs / point releases:

1. **CI gate for `.why.md` on caps.** A lightweight check that fails CI when a
   PR introduces a new throttle / cap / denylist without a co-located
   `.why.md`. Needs a definition of "cap" the grep can recognise. Plausible
   first cut: a `// rip-out-tax:` line comment that names the `.why.md`.
2. **Expired-trigger audit.** A periodic check that walks `.why.md` files,
   evaluates their removal triggers against current metrics, and files a
   finding when a cap should be retired. Belongs under ADR-011's audit
   subsystem.
3. **DEASR scoring stored as data, not prose.** Today the Scale-test table is
   markdown. A follow-up could give it a YAML twin so the substrate can query
   "every fix that ever scored *caps* on Users" — but the markdown table is
   the right shape for v1.
4. **Rename the workflow-engine `OnAudit` / `OnResolve` callbacks.** Tracked
   under [/CONTEXT.md § Known Terminology Collisions](../../CONTEXT.md#known-terminology-collisions);
   needs its own code-rename ADR.

---

## Status

This ADR is **Accepted**. It commits the project to D1–D6 above for new
process retrospectives. Existing retros stay in their current form.

Acceptance does NOT pre-authorise:

- Renaming the workflow-engine `OnAudit` / `OnResolve` callbacks (still
  collision 2b in CONTEXT.md; a separate ADR owns that rename).
- Renaming the `dear-agent` project or any `cmd/dear-agent-*` binary.
- A CI gate that enforces the rip-out tax (open question 1).
- Rewriting historical retros (append-only).

---

## References

- [/CONTEXT.md § DEASR](../../CONTEXT.md#deasr--diagnose--evaluate--scale-test--act--review)
- [AGENTS.why.md § rip-out-tax tracker](../../AGENTS.why.md)
- [.claude/CLAUDE.md § DEASR loop](../../.claude/CLAUDE.md)
- [docs/retros/_TEMPLATE.md](../retros/_TEMPLATE.md)
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md) — workflow-engine code lifecycle (not renamed)
- [ADR-018: Graceful Exit](ADR-018-graceful-exit-framework-default.md) — same two-tier (instruction + configuration) shape this ADR borrows
- 2026-05-13 enforcement-rules retro
  (`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`) —
  the turn-budget rejection that named the "training wheels that punish careful
  work" failure mode this ADR generalises.
