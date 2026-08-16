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

**Ownership:** this file owns vocabulary,
[MISSION.md](docs/alignment/MISSION.md) owns current project purpose and the
VROOM/AGM ownership boundary, and
[ADR-002](docs/adr/ADR-002-vroom-execution-architecture.md) owns the
architecture rationale.

---

## The Four Frameworks — and how they relate

These four names are easy to conflate. They operate at different levels and do
different jobs. The one-sentence relationship:

> **Wayfinder** plans the work, **VROOM** executes the work, **AGM** is the tool
> VROOM drives to run agent sessions, and **DEAR** is the retrospective loop that
> feeds lessons from finished work back into the plan.

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
            │  DEAR  — Define → Execute → Audit → Retro             │
            │  retrospective loop; findings flow back to Wayfinder/ │
            │  the roadmap (via the Meta-Orchestrator)              │
            └─────────────────────────────────────────────────────┘
```

| Term | Level | One-line definition |
|------|-------|---------------------|
| **Wayfinder** | Planning | The research / planning / prep phase that precedes execution. |
| **VROOM** | Execution | The supervisory framework that governs how agents do work. **Higher level than AGM.** |
| **AGM** | Tooling | Agent Gateway Manager — the CLI/runtime that actually spawns and drives agent sessions. A *tool VROOM uses*, not the framework itself. |
| **DEAR** | Process | Define → Execute → Audit → Retro — the per-task retrospective loop. |

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
> VROOM as the name of the framework.

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
| **Meta-Orchestrator** | CTO | Roadmap, prioritization, technology consistency, anti-duplication | Overseer | Orchestrator |
| **Orchestrator** | COO | Enqueue/dequeue work, monitor active workers, keep steady progress, never sit idle | Meta-Orchestrator | Overseer |
| **Overseer** | CRO (Reliability) | Resource usage (CPU/disk/memory/quota), leak detection, session cleanup | Orchestrator | Meta-Orchestrator |

The canonical code representation of this table lives in
`pkg/vroom/supervisor`: it owns the three session identities, compact aliases,
roles, and peer relationships. Launchers and AGM heartbeat tooling consume that
topology while retaining their own harness, model, scheduling, and persistence
policy.

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

**Auditors** examine logs, [DEAR](#dear--define--execute--audit--retro) retros,
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
| **Process — missing tools** | If an agent was not given a tool it needs, that is a **bug**: run a [DEAR](#dear--define--execute--audit--retro) retro and grant the right permissions to that worker role. Track these; if enough get approved, grant by default (don't burn tokens re-approving). |
| **Process — needs access it lacks** | The agent asks its **manager** (whoever spawned it), who approves or denies. Track the pattern via DEAR retros and periodic audits. |
| **AskUserQuestion** | Worker unsure → asks its **manager** (the Requester that spawned it) → if the manager is confident it answers directly, else it escalates up the management chain → terminating at the **Meta-Orchestrator**, which asks the human if still unsure. (Note: "manager" is the spawn/Requester relationship, distinct from the three named *supervisor roles* — the chain follows who-spawned-whom and ends at the Meta-Orchestrator, so it cannot loop on the supervisors' cyclic Secondary mesh.) **Blocking for the Worker** (it needs the answer); **non-blocking for the manager/supervisor roles** (they supervise many sessions and must not stall the whole pipeline on one question). |
| **Proposing work** | Anyone may propose work via a formal [Work Order](#work-order). It goes to the Meta-Orchestrator. |

### Lifecycle summary

- **Supervisors** run continuously while there is backlog and/or active enqueued
  work. With active work they periodically check progress; problems trigger a
  DEAR retro → remediation → an enqueued long-term fix. Spare capacity → spin up
  more Workers.
- **Workers** do the work, report back to their **manager** (the Requester
  that spawned them), may ask questions upward, and on completion run a DEAR
  retro whose findings feed the backlog/roadmap (filtered through the
  Meta-Orchestrator).
- **Auditors** trigger periodically; results feed the Meta-Orchestrator.
- **SREs** triage fire vs. merely-slow process. If it can wait, they say so and
  fix nothing. If it is a real fire, they act.

### Decision trail

Consequential VROOM decisions are recorded to an append-only decision trail.
The append-only persistence lives in `pkg/vroom/decisiontrail`; the in-memory
event topics live in `pkg/vroom/vroom`.

---

## AGM — Agent Gateway Manager

**AGM is a tool.** It is the CLI/runtime that spawns, messages, monitors, and
reaps agent sessions (tmux-backed), provides the message queue, hooks, and
workspace model. It lives under `agm/`.

AGM is what VROOM *drives*. The distinction matters: "the Orchestrator dispatches
a Worker" is a VROOM statement; "`agm session new` starts a tmux session" is an
AGM statement. AGM has no opinion about roadmaps, prioritization, or supervisory
roles — that is all VROOM.

The exact VROOM/AGM ownership contract lives in the canonical
[`MISSION.md`](docs/alignment/MISSION.md); this vocabulary guide does not restate
it. AGM's `session verify` and `batch verify` commands report whether supplied
assertions pass, and the mission defines how VROOM uses that evidence.

AGM has its own internal ADRs under `agm/docs/adr/` and `agm/cmd/.../adr/`. Those
are legitimately AGM-scoped. **Cross-cutting / above-AGM architecture (like
VROOM) belongs in the top-level [`docs/adr/`](docs/adr/), not under `agm/`.**

---

## DEAR — Define → Execute → Audit → Retro

**DEAR is the per-task retrospective loop.** Canonical expansion in this repo's
**process/governance** vocabulary:

| Letter | Phase | Meaning |
|--------|-------|---------|
| **D** | **Define** | State the task and its exit conditions (acceptance criteria). |
| **E** | **Execute** | Do the work. |
| **A** | **Audit** | Verify the runnable exit conditions actually hold. |
| **R** | **Retro** | Retrospective: capture what was learned; findings flow to the backlog/roadmap via the Meta-Orchestrator. |

This is the expansion used by `AGENTS.md` and by the VROOM lifecycle above.
The canonical disambiguation is
[ADR-035](docs/adr/ADR-035-dear-terminology-disambiguation.md): bare "DEAR"
means this process loop unless a document is quoting a historical identifier.

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
| **Primary / Secondary / Tertiary** | The three-agent ownership of any one task: doer / verifier-and-watcher / unsticker. _Avoid: Lead/Support/Backup (obscures the doer→verifier→unsticker chain); Owner (implies authority, not task ownership)._ |
| **Meta-Orchestrator** | CTO supervisor. Sole roadmap-add authority. Secondary: Overseer; Tertiary: Orchestrator. _Avoid: Tech Lead, Architect (these are analogies only, not synonyms)._ |
| **Orchestrator** | COO supervisor. Work queue + worker monitoring + progress. Secondary: Meta-Orchestrator; Tertiary: Overseer. _Avoid: Scheduler, Dispatcher (loses the progress-monitoring/COO dimension)._ |
| **Overseer** | CRO supervisor. Resource/leak/cleanup reliability. Secondary: Orchestrator; Tertiary: Meta-Orchestrator. _Avoid: Monitor, Watchdog (these miss the reliability/CRO framing)._ |
| **Worker** | Does one task, hands back for verification. Secondary = its Requester; Tertiary = the Requester's Secondary. _Avoid: Agent (too generic — every participant is an agent); Bot._ |
| **Requester** | *Relationship, not a standing role:* the agent that spawned a given Worker. _Avoid: Spawner, Caller (obscures that this is a dynamic relationship, not a standing role)._ |
| **Auditor** | Periodically mines logs/DEAR retros for patterns; feeds the Meta-Orchestrator. _Avoid: Reviewer (implies code review, not log/retro pattern mining)._ |
| **SRE agent** | Emergency-only, privileged firefighter. First decides if there is a real fire. _Avoid: On-call agent, Emergency responder (misses the "first verify it's a real fire" rule that defines this role)._ |
| **Work Order** | <a id="work-order"></a>The formal artifact for proposing work. Required fields include **the reason the work should be added**. Routed to the Meta-Orchestrator, who alone may add it to the roadmap. _Avoid: Task, Ticket, Issue (these lack the mandatory reason field and the formal routing; Work Order is a specific artifact, not a generic tracker item)._ |
| **AGM** | Agent Gateway Manager — the session-driving tool VROOM uses. _Avoid: VROOM (AGM is a tool VROOM drives; conflating the two erases the framework/tool distinction — a known collision in pre-2026-05-17 docs)._ |
| **VROOM** | The supervisory execution framework (proper name; old V/R/O/O/M backronym retired). _Avoid: AGM (VROOM is the framework above AGM, not the runtime); Orchestration layer (too vague, erases the three-supervisor structure)._ |
| **DEAR** | Define → Execute → Audit → Retro retrospective loop (process sense). _Avoid: Post-mortem, Sprint retro (DEAR covers all four phases; "retro" alone names only the R step); use "workflow lifecycle hooks" for the workflow-engine Define/Enforce/Audit/Resolve API._ |
| **Wayfinder** | The research/planning/prep phase preceding execution. _Avoid: "Planning" alone (too generic); Pre-work (undersells the structured research/SDLC scope)._ |

---

## Known Terminology Collisions

These are or were **bugs in the vocabulary**, recorded here per the "call
conflicts out immediately" rule. Some entries are resolved by ADRs and remain
listed so older docs and identifiers can be interpreted correctly.

1. **"DEAR" — resolved two-level model plus historical identifiers.**
   - **(a) Process / retrospective loop:** Define → **Execute** → Audit →
     **Retro**. *Bare "DEAR" means this process loop.*
   - **(b) Workflow lifecycle hooks:** `pkg/workflow.Hooks` and ADR-010/ADR-011
     use **Define → Enforce → Audit → Resolve & Refine** for the workflow-engine's
     `OnDefine/OnEnforce/OnAudit/OnResolve` callbacks. This is a *code concept*
     with a different "E" and "R"; docs call it "workflow lifecycle hooks", not
     "DEAR hooks". Exported Go names stay unchanged.
   - **(c) Archived backlog prefix:** legacy `DEAR-X.*` identifiers were a
     numbering convention for framework-improvement items, unrelated to either
     loop. Current work uses Beads IDs.
   Canonical authority: [ADR-035](docs/adr/ADR-035-dear-terminology-disambiguation.md).

2. **ADR sprawl (~70 ADRs of mixed quality).**
   Many ADRs in the repo fail the "hard-to-reverse + surprising + real
   trade-off" test in [docs/adr/README.md](docs/adr/README.md) (bug-fix notes,
   standard-pattern conventions, LLM-padded design dumps). Per-ADR dispositions
   live in `vbonnet/engram-research`
   `audits/2026-05-17-adr-inventory-prune.md`; remediation is tracked in bead
   `ce-g62j` (with `ce-y3zh.3`).

---

## When you find a term that disagrees with this file

Fix the other document, not this file — unless the disagreement reveals that
*this file* is wrong, in which case update this file in the same change. Do not
let two definitions coexist silently.
