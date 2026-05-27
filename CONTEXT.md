# CONTEXT.md — dear-agent Ubiquitous Language

This file is the **single source of truth for vocabulary** in this repository.
Agents and humans use these terms with exactly these meanings. When a document,
ADR, comment, or commit message uses one of these terms, it means what is
written here — not some adjacent thing.

Inspired by the DDD "ubiquitous language" pattern (cf. Matt Pocock's
`grill-with-docs`): terms must be **domain-meaningful**, not coupled to a
particular implementation, and **conflicts are called out immediately** rather
than papered over. Where a term currently has more than one meaning in this
codebase, that collision is documented in
[§ Known Terminology Collisions](#known-terminology-collisions) and treated as a
bug to be paid down, not a fact to live with.

> **Status note (2026-05-17):** This file was created alongside a correction of
> the VROOM architecture docs. Several pre-existing documents described an
> earlier, inaccurate VROOM model (a five-role "Verifier/Requester/Orchestrator/
> Overseer/Meta-Orchestrator" mesh with a lexicographic value evaluator). That
> model is **superseded**. This file and
> [docs/adr/ADR-002](docs/adr/ADR-002-vroom-execution-architecture.md) are
> authoritative; anything that disagrees is stale and should be fixed to match.

---

## The Four Frameworks — and how they relate

These four names are easy to conflate. They operate at different levels and do
different jobs. The one-sentence relationship:

> **Wayfinder** plans the work, **VROOM** executes the work, **AGM** is the tool
> VROOM drives to run agent sessions, and **DEASR** is the retrospective loop
> that feeds lessons from finished work back into the plan — under the
> **push-bike, not training wheels** design constraint.

```
            ┌─────────────────────────────────────────────────────┐
            │  WAYFINDER  — research / planning / prep phase        │
            │  "understand the problem before touching code"        │
            └───────────────────────┬─────────────────────────────┘
                                     │ produces plans / roadmap input
                                     ▼
            ┌─────────────────────────────────────────────────────┐
            │  VROOM  — the execution framework (supervisory mesh)  │
            │  3 supervisors + Workers + Auditors + SREs            │
            └───────────────────────┬─────────────────────────────┘
                                     │ drives, as one of its tools
                                     ▼
            ┌─────────────────────────────────────────────────────┐
            │  AGM  — Agent Gateway Manager (a tool)                │
            │  spawns / messages / monitors agent sessions          │
            └───────────────────────┬─────────────────────────────┘
                                     │ every finished unit of work runs
                                     ▼
            ┌─────────────────────────────────────────────────────┐
            │  DEASR  — Diagnose → Evaluate → scAle-test → Act →    │
            │           Review                                      │
            │  retrospective loop; findings flow back to Wayfinder/ │
            │  the roadmap (via the Meta-Orchestrator). Push-bike,  │
            │  not training wheels.                                 │
            └─────────────────────────────────────────────────────┘
```

| Term | Level | One-line definition |
|------|-------|---------------------|
| **Wayfinder** | Planning | The research / planning / prep phase that precedes execution. |
| **VROOM** | Execution | The supervisory framework that governs how agents do work. **Higher level than AGM.** |
| **AGM** | Tooling | Agent Gateway Manager — the CLI/runtime that actually spawns and drives agent sessions. A *tool VROOM uses*, not the framework itself. |
| **DEASR** | Process | Diagnose → Evaluate → scAle-test → Act → Review — the per-task retrospective loop, governed by the **push-bike, not training wheels** design constraint. (Successor to "DEAR"; see [ADR-024](docs/adr/ADR-024-deasr-push-bike-philosophy.md).) |

---

## VROOM — the execution framework

**VROOM is the supervisory framework under which AI agents collaborate on
software-engineering tasks.** It sits **above AGM**: AGM is one of the tools
VROOM uses to spawn and drive sessions. VROOM is *not* an AGM-internal concept,
and documents that file it under AGM (e.g. `agm/docs/...`) are misfiled.

> **"VROOM" is a proper name, not an acronym.** It was historically
> backronymed **V**erifier / **R**equester / **O**rchestrator / **O**verseer /
> **M**eta-Orchestrator. That five-role expansion is **inaccurate and
> superseded** — there is no "Verifier" or "Requester" *supervisor* role. Treat
> VROOM as the name of the framework. (This was a judgment call made while
> correcting the docs; if a canonical expansion is desired, the maintainer can
> set one and this note becomes the place to record it.)

### Per-task responsibility: Primary / Secondary / Tertiary

Every task is owned by three agents:

| Role | Responsibility |
|------|----------------|
| **Primary** | Getting the task done. |
| **Secondary** | Making sure the Primary gets it done, and **verifying the output**. |
| **Tertiary** | Making sure the Primary and Secondary are doing their jobs, and **unsticking them** when they are not. |

"Verification" is therefore a *responsibility of the Secondary*, not a separate
standing role. (This is why the old "Verifier role" is gone — it was a
responsibility, not a supervisor.)

### The three supervisors

Three long-lived supervisor agents each run a **constant loop**. The **first
action of every loop iteration** is to run the supervisor-check skill against
the *other two* supervisors and unblock them if needed. (The most common
blockage observed has been permission-prompt errors; this may be largely moot
under Auto permission mode.) Only after that do they do their own job.

| Supervisor | "C-suite" analogy | Owns | Secondary | Tertiary |
|------------|-------------------|------|-----------|----------|
| **Meta-Orchestrator** | CTO | Roadmap, prioritization, technology consistency, not reinventing the wheel | Overseer | Orchestrator |
| **Orchestrator** | COO | Enqueue/dequeue work, monitor active workers, keep steady progress, never sit idle | Meta-Orchestrator | Overseer |
| **Overseer** | CRO (Reliability) | Resource usage (CPU/disk/memory/quota), leak detection, session cleanup | Orchestrator | Meta-Orchestrator |

**Roadmap authority:** The **Meta-Orchestrator is the only agent allowed to add
items to the roadmap.** Every other agent may *propose* work (see
[Work Order](#work-order)); the Meta-Orchestrator has final say. This is the
mechanism that prevents duplicate or divergent work. Meta-Orchestrator
acceptance questions: Does this fit the vision/values? Does it overlap or
conflict with existing work? Should scope expand (e.g. fix *every* instance of a
bug class, not one)?

### Workers

A **Worker** does one assigned task, then hands it back for verification.

- Spawned mostly by the Orchestrator, but **any agent may spawn a Worker**.
- Worker roles: coder, code reviewer, researcher, writer, etc.
- A Worker's **Secondary is the Requester** (the agent that spawned it).
- A Worker's **Tertiary is the Secondary of that Requester.**

### Auditors

**Auditors** examine logs, [DEASR](#deasr--diagnose--evaluate--scale-test--act--review) retros,
and other records to find patterns that need addressing. Findings loop back to
the roadmap (through the Meta-Orchestrator) and are prioritized by severity.
Auditors trigger periodically.

### SRE Agents

**SRE agents** are special-case firefighters, **spawned only in emergencies**.

- **First job: determine whether there is actually a fire.** If not, the correct
  response is "not my job" — use the standard process/escalation paths instead.
- Given **privileged access**. Precisely because of that, an SRE **may not be
  spawned by another agent to bypass rules**; standard escalations exist for
  that.
- A real fire = security incident, runaway process, leaked token/credential,
  financial leak, or similar. In a real fire the SRE may do whatever is
  necessary within legal/sanity bounds.
- SRE remediation must hold **long enough for a real root-cause fix** to land —
  not a one-time duct-tape patch that silently rots.

### Projects / scaling

A deployment may eventually need **multiple supervisor sets**
(Meta-Orchestrator/Orchestrator/Overseer) — e.g. parallel workstreams like the
dear-agent repo, personal projects, brain-v2. Supervisor sets are **scoped to
AGM workspaces**. Start with a single set; expand only if necessary.

### Escalations

| Kind | Rule |
|------|------|
| **Process — missing tools** | If an agent was not given a tool it needs, that is a **bug**: run a [DEASR](#deasr--diagnose--evaluate--scale-test--act--review) retro and grant the right permissions to that worker role. Track these; if enough get approved, grant by default (don't burn tokens re-approving). |
| **Process — needs access it lacks** | The agent asks its **manager** (whoever spawned it), who approves or denies. Track the pattern via DEASR retros and periodic audits. |
| **AskUserQuestion** | Worker unsure → asks its **manager** (the Requester that spawned it) → if the manager is confident it answers directly, else it escalates up the management chain → terminating at the **Meta-Orchestrator**, which asks the human if still unsure. (Note: "manager" is the spawn/Requester relationship, distinct from the three named *supervisor roles* — the chain follows who-spawned-whom and ends at the Meta-Orchestrator, so it cannot loop on the supervisors' cyclic Secondary mesh.) **Blocking for the Worker** (it needs the answer); **non-blocking for the manager/supervisor roles** (they supervise many sessions and must not stall the whole pipeline on one question). |
| **Proposing work** | Anyone may propose work via a formal [Work Order](#work-order). It goes to the Meta-Orchestrator. |

### Lifecycle summary

- **Supervisors** run continuously while there is backlog and/or active enqueued
  work. With active work they periodically check progress; problems trigger a
  DEASR retro → remediation → an enqueued long-term fix. Spare capacity → spin
  up more Workers.
- **Workers** do the work, report back to their **manager** (the Requester
  that spawned them), may ask questions upward, and on completion run a DEASR
  retro whose findings feed the backlog/roadmap (filtered through the
  Meta-Orchestrator).
- **Auditors** trigger periodically; results feed the Meta-Orchestrator.
- **SREs** triage fire vs. merely-slow process. If it can wait, they say so and
  fix nothing. If it is a real fire, they act.

### Decision trail

Consequential VROOM decisions are recorded to an append-only decision trail.
This concept is sound and survives the model correction. ⚠️ The current code
seam (`pkg/vroom`) still encodes the *old* role set — see
[Known Terminology Collisions](#known-terminology-collisions).

---

## AGM — Agent Gateway Manager

**AGM is a tool.** It is the CLI/runtime that spawns, messages, monitors, and
reaps agent sessions (tmux-backed), provides the message queue, hooks, and
workspace model. It lives under `agm/`.

AGM is what VROOM *drives*. The distinction matters: "the Orchestrator dispatches
a Worker" is a VROOM statement; "`agm session new` starts a tmux session" is an
AGM statement. AGM has no opinion about roadmaps, prioritization, or supervisory
roles — that is all VROOM.

AGM has its own internal ADRs under `agm/docs/adr/` and `agm/cmd/.../adr/`. Those
are legitimately AGM-scoped. **Cross-cutting / above-AGM architecture (like
VROOM) belongs in the top-level [`docs/adr/`](docs/adr/), not under `agm/`.**

---

## DEASR — Diagnose → Evaluate → scAle-test → Act → Review

**DEASR is the per-task retrospective loop.** Canonical expansion in this
repo's **process/governance** vocabulary:

| Letter | Phase           | Meaning |
|--------|-----------------|---------|
| **D**  | **Diagnose**    | State the task / incident, **current scale**, **target scale**, and the **scaling model** that connects them. A retro that cannot fill the scale block is sizing a fix to unmeasured load. |
| **E**  | **Evaluate**    | Two passes, in order: (1) describe the **ideal scalable solution first** — no reference to current code or this-week constraints; (2) describe the **minimum** we can ship now that is **on the path to the ideal**. If the minimum has to be ripped out to reach the ideal, reject it and restart at (1). |
| **A**  | **scAle-test** (Assay) | Score every proposed fix against three axes — **10× agents**, **10× machines**, **10× users** — as **scales** / **neutral** / **caps**. A fix scoring `caps` on any axis MUST carry a co-located `.why.md` declaring the **rip-out tax** (see below). |
| **S**  | (S)cope-down → Act | Build the path-compatible minimum from Evaluate (2). Every Act item must appear in the Scale-test table. |
| **R**  | **Review**      | Capture what was learned; findings flow to the backlog/roadmap via the Meta-Orchestrator. |

> **Push-bike, not training wheels** — the design constraint that governs
> every DEASR retro.
>
> A **push-bike** is the simplest shape of the *eventual* bike — frame,
> wheels, no pedals — that a child rides on the path to a real bike.
> Everything they learn (balance, steering, momentum) transfers. Nothing has
> to be ripped out when pedals arrive.
>
> **Training wheels** are bolted-on caps that prevent the child from learning
> the thing that actually matters (balance) and must be removed before they
> can ride for real. Every training-wheel solution buys time at the cost of
> teaching the wrong reflex.
>
> Prefer push-bike solutions. When a training-wheel solution is the only
> option this week, **declare it as one** (rip-out tax, below) and schedule
> its removal.
>
> Citing "push-bike, not training wheels" in a review is a legitimate veto —
> it forces the proposer to either reframe the fix or attach a rip-out tax.

### Rip-out tax — the `.why.md` for every cap

Any solution that scores `caps` on any axis MUST declare, in the same change,
a `.why.md` file co-located with the code it caps. This reuses the existing
`.why.md` decision-log pattern (see [AGENTS.why.md](AGENTS.why.md)) — caps
are deliberate decisions deserving co-located rationale, not silent
constants. The file states:

- **Type:** cap (rip-out tax)
- **Why this is here:** the load it relieves, the regression it prevents
- **What it costs to remove:** code touched, migration shape
- **Removal trigger:** the observable signal that says "now safe to remove"
- **Related retro:** link to the DEASR retro that introduced it

### Where this lives

- **Process source of truth:** this section + [ADR-024](docs/adr/ADR-024-deasr-push-bike-philosophy.md).
- **Retro template:** [docs/retros/_TEMPLATE.md](docs/retros/_TEMPLATE.md).
- **Agent instruction:** [.claude/CLAUDE.md § DEASR loop](.claude/CLAUDE.md).
- **Historical retros** under `docs/retros/` are not retroactively renamed
  (append-only audit trail per `docs/alignment/VALUES.md`); new retros use
  the DEASR template.

⚠️ The letters **DEASR** are distinct from the workflow-engine *code*
lifecycle (`OnDefine` / `OnEnforce` / `OnAudit` / `OnResolve` — historically
also abbreviated "DEAR"); they no longer collapse to the same acronym. The
remaining naming collision is documented in
[Known Terminology Collisions](#known-terminology-collisions).

---

## Wayfinder — research / planning / prep phase

**Wayfinder is the phase that happens before execution:** understand the
affected code, clean up (TODOs, dead code, naming), add missing tests, refactor
for clarity — *then* implement ("think before coding"). It also covers code
review and SDLC workflow tooling. Ships under `wayfinder/`.

Relationship to VROOM: Wayfinder is **planning**, VROOM is **execution**. Output
of Wayfinder feeds the roadmap that the Meta-Orchestrator owns.

---

## Glossary (quick reference)

| Term | Definition |
|------|------------|
| **Primary / Secondary / Tertiary** | The three-agent ownership of any one task: doer / verifier-and-watcher / unsticker. |
| **Meta-Orchestrator** | CTO supervisor. Sole roadmap-add authority. Secondary: Overseer; Tertiary: Orchestrator. |
| **Orchestrator** | COO supervisor. Work queue + worker monitoring + progress. Secondary: Meta-Orchestrator; Tertiary: Overseer. |
| **Overseer** | CRO supervisor. Resource/leak/cleanup reliability. Secondary: Orchestrator; Tertiary: Meta-Orchestrator. |
| **Worker** | Does one task, hands back for verification. Secondary = its Requester; Tertiary = the Requester's Secondary. |
| **Requester** | *Relationship, not a standing role:* the agent that spawned a given Worker. |
| **Auditor** | Periodically mines logs/DEASR retros for patterns; feeds the Meta-Orchestrator. |
| **SRE agent** | Emergency-only, privileged firefighter. First decides if there is a real fire. |
| **Work Order** | <a id="work-order"></a>The formal artifact for proposing work. Required fields include **the reason the work should be added**. Routed to the Meta-Orchestrator, who alone may add it to the roadmap. |
| **AGM** | Agent Gateway Manager — the session-driving tool VROOM uses. |
| **VROOM** | The supervisory execution framework (proper name; old V/R/O/O/M backronym retired). |
| **DEASR** | Diagnose → Evaluate → scAle-test → Act → Review retrospective loop (process sense). Successor to "DEAR"; see [ADR-024](docs/adr/ADR-024-deasr-push-bike-philosophy.md). |
| **Push-bike, not training wheels** | Design constraint applied across DEASR: prefer fixes that teach the system the right reflex at target scale over bolted-on caps that have to be removed later. See [DEASR § Push-bike, not training wheels](#deasr--diagnose--evaluate--scale-test--act--review). |
| **Rip-out tax** | The co-located `.why.md` that every `caps`-scored fix MUST carry, declaring removal cost, code touched, and removal trigger. Same pattern as [AGENTS.why.md](AGENTS.why.md) decision logs. |
| **Wayfinder** | The research/planning/prep phase preceding execution. |

---

## Known Terminology Collisions

These are **bugs in the vocabulary**, recorded here per the "call conflicts out
immediately" rule. They are not resolved by this document alone; resolving them
needs follow-up work tracked on the roadmap.

1. **"VROOM" — old five-role model vs. corrected model.**
   Many pre-2026-05-17 docs describe VROOM as Verifier/Requester/Orchestrator/
   Overseer/Meta-Orchestrator with a lexicographic value evaluator. That model
   is superseded by the one in this file and
   [docs/adr/ADR-002](docs/adr/ADR-002-vroom-execution-architecture.md). The
   superseded ADRs (`agm/docs/adr/ADR-020`…`ADR-025`) are stubbed with redirects.

2. **"DEAR" — historically three meanings; one resolved by [ADR-024](docs/adr/ADR-024-deasr-push-bike-philosophy.md).**
   - **(a) Process / retrospective loop — RENAMED to "DEASR" (2026-05-26).**
     Was Define → Execute → Audit → Retro; now **Diagnose → Evaluate →
     scAle-test → Act → Review** under the **push-bike, not training wheels**
     design constraint. See [§ DEASR](#deasr--diagnose--evaluate--scale-test--act--review)
     and [ADR-024](docs/adr/ADR-024-deasr-push-bike-philosophy.md). Historical
     retros under `docs/retros/` keep their "DEAR Retro:" titles (append-only
     audit trail); new retros use the DEASR template.
   - **(b) Workflow-engine code lifecycle hooks — UNCHANGED:**
     `pkg/workflow.Hooks` and ADR-010/ADR-011 use **Define → Enforce → Audit →
     Resolve & Refine** for the `OnDefine/OnEnforce/OnAudit/OnResolve`
     callbacks. The acronym collision is **smaller** now (DEASR vs.
     workflow-engine DEAR — they no longer expand to the same string), but the
     code-side name "DEAR" persists. Renaming exported callback names is a
     hard-to-reverse API change deserving its own ADR; tracked as a follow-up.
   - **(c) Backlog phase prefix:** `DEAR-X.*` identifiers in
     `docs/workflow-engine/BACKLOG.md` / ROADMAP are a numbering convention for
     framework-improvement items, unrelated to either loop. Untouched —
     renaming established issue IDs would break inbound references for no
     benefit.
   *Resolution owner:* Meta-Orchestrator (roadmap). Remaining work: rename the
   code-level lifecycle so it stops shadowing the historical "DEAR" name.

3. **`pkg/vroom` code encodes the superseded model.**
   `pkg/vroom/vroom/topics.go` defines decision-trail topics
   (`vroom.decision.evaluated` with a "Verifier" comment, etc.) tied to
   "ADR-020's decision trail specification". The decision-trail *idea* is fine;
   the *role enum* is stale. Not changed in the docs PR (breaking API change) —
   tracked as a follow-up.

4. **Two top-level ADR directories — RESOLVED (2026-05-17).**
   There used to be both `docs/adr/` (singular) and `docs/adrs/` (plural).
   They are now **consolidated into `docs/adr/`** (the conventional name, also
   used by `agm/docs/adr/`). The canonical top-level ADR directory is
   **`docs/adr/`**; `docs/adrs/` no longer exists. ADR numbers were left
   unchanged (gaps are fine; renumbering would break inbound references and
   ADR identity). Nested per-package dirs like `pkg/engram/docs/adrs/` are a
   separate concern and were intentionally left alone.

5. **ADR sprawl (~100+ ADRs of mixed quality).**
   Most ADRs in the repo fail the "hard-to-reverse + surprising + real
   trade-off" test (many are bug-fix notes, standard-pattern conventions, or
   LLM-padded design dumps). A full repo-wide audit with per-ADR dispositions
   is in [docs/audits/2026-05-17-adr-inventory-prune.md](docs/audits/2026-05-17-adr-inventory-prune.md).
   Only the top-level governance set + one exact duplicate are pruned in the
   originating PR; the rest are grouped into follow-up surgical PRs.

---

## When you find a term that disagrees with this file

Fix the other document, not this file — unless the disagreement reveals that
*this file* is wrong, in which case update this file in the same change and note
it in the status line at the top. Do not let two definitions coexist silently.
