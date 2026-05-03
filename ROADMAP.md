# dear-agent Roadmap

**Status:** Active
**Last updated:** 2026-05-03
**Owner:** vbonnet
**Source of truth for:** what work needs to happen on the workflow engine, in
what order, and who/what is currently doing it.

This roadmap is the contract between sessions. Any agent picking up work on
the workflow engine should read this file first to know what phase is in
flight, what tickets are open, and which decisions are already made.

---

## How this file works

- **Phases** are the unit of release. Each phase is independently shippable,
  has clear deliverables, and a single `Status:` line. Read those to know
  where the project is.
- **Tickets** under each phase are the unit of work. Each ticket has an `id`,
  a one-line description, files it touches, acceptance criteria, dependencies,
  and a status. Pick up the next `pending` ticket whose deps are `done`.
- **Status values:**
  - `pending` — not started.
  - `in-flight` — actively being worked on. Note the session/branch.
  - `blocked` — waiting on something. Note the blocker.
  - `done` — merged to main. Note the PR number.
- **Updating:** when you start a ticket, change its status to `in-flight` and
  add a `Session:` line. When you merge, mark it `done` and add the PR. Keep
  the diffs to this file small and frequent.

---

## Foundation: the research

The workflow engine direction comes from four sibling research documents
landed on `engram-research/main` on 2026-05-02:

| Doc | Angle | Path on `engram-research` |
|---|---|---|
| `WORKFLOW-ENGINE-RESEARCH-CLAUDE.md` | Substrate / DEAR alignment | `/` |
| `WORKFLOW-ENGINE-RESEARCH-ENGINEERING.md` | Schemas, APIs, schema, tickets | `/` |
| `WORKFLOW-ENGINE-RESEARCH-ECOSYSTEM.md` | Framework comparison, DX | `/` |
| `WORKFLOW-ENGINE-SYNTHESIS.md` | Unified architecture + 6-phase plan | `/` |

To read on demand:

```bash
git -C ~/src/engram-research show origin/main:WORKFLOW-ENGINE-SYNTHESIS.md
git -C ~/src/engram-research show origin/main:WORKFLOW-ENGINE-RESEARCH-ENGINEERING.md
git -C ~/src/engram-research show origin/main:WORKFLOW-ENGINE-RESEARCH-CLAUDE.md
git -C ~/src/engram-research show origin/main:WORKFLOW-ENGINE-RESEARCH-ECOSYSTEM.md
```

The architectural decisions are captured in
[`docs/adrs/ADR-010-workflow-engine-architecture.md`](docs/adrs/ADR-010-workflow-engine-architecture.md).
The per-ticket backlog (one row per work item) is at
[`docs/workflow-engine/BACKLOG.md`](docs/workflow-engine/BACKLOG.md). Read both
before picking up work.

---

## The thesis (one paragraph)

`pkg/workflow` evolves from "a YAML DAG runner" into **the substrate-quality
work-item layer** that the substrate hypothesis (ADR-009) said was missing.
The engine and the work-item store are the same thing. Every node-execution
becomes a record: durable, owned, state-machined, audited, permissioned. AGM
holds process state; Engram holds knowledge state; the workflow engine holds
*the work itself*. Three packages, one SQLite database, additive changes only.

---

## Architecture at a glance

```
                 ┌──────────────────────────────────────────────┐
                 │                AGM (sessions)                │
                 │     process state, sandboxes, harnesses      │
                 └────────────────┬─────────────────────────────┘
                                  │ executes nodes inside
                                  ▼
   ┌────────────────────────────────────────────────────────────────┐
   │                  Workflow Engine  (NEW SUBSTRATE)              │
   │                                                                │
   │  pkg/workflow            (extended; additive only)             │
   │  pkg/workflow/roles/     (new: role registry + resolver)       │
   │  pkg/source/             (new: FetchSource / AddSource)        │
   │                                                                │
   │  SQLite tables: workflows, runs, nodes, node_attempts,         │
   │                 node_outputs, audit_events, approvals          │
   └─────────┬───────────────────────────────────┬──────────────────┘
             │ outputs.durability=engram_indexed │
             ▼                                   ▼
   ┌────────────────────────────────┐  ┌──────────────────────────┐
   │         pkg/source             │  │   Role registry          │
   │   (SQLite default; pluggable)  │  │   roles.yaml             │
   └────────────────────────────────┘  └──────────────────────────┘
                  ▲
                  │
   ┌──────────────┴──────────────────────────────────────────────┐
   │                        Engram                                │
   └──────────────────────────────────────────────────────────────┘
```

Full diagram and justification: [ADR-010](docs/adrs/ADR-010-workflow-engine-architecture.md).

---

## Minimum Viable Substrate (MVS) — the first milestone

**MVS = Phase 0 + Phase 1.** This is the smallest cut that satisfies all five
substrate properties (durability, ownership, state machine, audit,
permissions) by default. Anything below this line is "nice to have"; anything
above must ship together for the substrate framing to hold.

**Above the line (must ship together as `pkg/workflow v0.2`):**

1. **SQLite state.** `runs`, `nodes`, `node_attempts`, `audit_events` tables.
   Replaces `FileState` JSON for new runs. Old snapshots migrate via
   `workflow migrate`.
2. **Audit emission.** Every state transition writes a row. Default sink is
   the SQLite DB; user can attach stdout/JSONL/Engram.
3. **Role registry.** `roles.yaml`, primary/secondary/tertiary tier
   resolution, `dear-agent roles list/describe/validate`.
4. **`role:` field on AI nodes** (with `model:` fallback for back-compat).
5. **Budget enforcement.** `max_tokens`, `max_dollars`, `max_wallclock` per
   node; live print in CLI; `on_overrun: escalate|fail|truncate`.
6. **`workflow status / list / logs / cancel` CLI** reading from SQLite.

**Below the line (post-MVS, ship as separate phases 2-5):**

- HITL with first-class state + approver_role + Discord/MCP backends.
- Exit gates (5 kinds).
- Structured `outputs[]` with durability tiers.
- Permission enforcement (allowlists).
- FetchSource/AddSource MCP + SQLite adapter.
- `workflow dev` interactive mode.
- `kind: spawn` for emergent DAG growth.
- Obsidian / llm-wiki / OpenViking adapters.

**Why this cut:** the MVS gives the project's three load-bearing moves
(substrate-quality state + role abstraction + cost control) in one release.
Without these three, no other improvement matters. With them, the rest can
ship incrementally without breaking anything.

---

## Phases

| # | Name | Size | Status | Tracking |
|---|------|------|--------|----------|
| 0 | SQLite + audit_events (MVS pt. 1) | 4 weeks | `done (#38)` | [BACKLOG §0](docs/workflow-engine/BACKLOG.md#phase-0--sqlite--audit_events-mvs-pt-1) |
| 1 | Roles + budget (MVS pt. 2) | 3 weeks | `done` | [BACKLOG §1](docs/workflow-engine/BACKLOG.md#phase-1--roles--budget-mvs-pt-2) |
| 2 | DEAR hooks + HITL + exit_gate + outputs[] | 4 weeks | `done (#40)` | [BACKLOG §2](docs/workflow-engine/BACKLOG.md#phase-2--dear-hooks--hitl--exit_gate--outputs) |
| 3 | FetchSource / AddSource | 2 weeks | `done` | [BACKLOG §3](docs/workflow-engine/BACKLOG.md#phase-3--fetchsource--addsource) |
| 4 | Migration + `workflow dev` | 4 weeks | `pending` | [BACKLOG §4](docs/workflow-engine/BACKLOG.md#phase-4--migration--workflow-dev) |
| 5 | Adapters + visual inspector + `kind: spawn` | open-ended | `pending` | [BACKLOG §5](docs/workflow-engine/BACKLOG.md#phase-5--adapters--visual-inspector--kind-spawn) |

### Parallelism map

```
   Phase 0 ──┬──> Phase 1 ──> Phase 2 ──┬──> Phase 4 ──> Phase 5
             │                          │
             └─────> Phase 3 ───────────┘
```

Phases 0 and 3 can ship in parallel by different ICs (run state vs. output
state). Phase 4 unlocks once 1+2+3 land. Phase 5 is open-ended.

---

## Phase summaries

### Phase 0 — SQLite + audit_events (MVS pt. 1)

**Goal:** every node-execution is a queryable row, every state transition is
an audit event. Existing workflows run unchanged.

**Deliverables:**

- `SQLiteState` implementing the existing `State` interface, with WAL mode.
- New tables: `workflows`, `runs`, `nodes`, `node_attempts`, `audit_events`.
- Runner emits a row per attempt and per transition.
- CLI: `workflow status / list / logs / cancel`.
- Migration tool: old JSON snapshot → SQLite row.

**Ship criterion:** existing workflows run unchanged; `SELECT * FROM
audit_events WHERE run_id = ?` returns every transition; perf targets met
(see [ADR-010 §6](docs/adrs/ADR-010-workflow-engine-architecture.md#6-performance-targets)):
read run status for 100-node DAG P95 < 5 ms; append audit event P95 < 1 ms.

### Phase 1 — Roles + budget (MVS pt. 2)

**Goal:** AI nodes declare `role:` not `model:`. A central registry resolves
the role to a model tier. Budgets are enforced per-node and per-run.

**Deliverables:**

- `pkg/workflow/roles/{registry,resolver}.go`.
- `roles.yaml` loader; tier resolution; capacity/cost/capability filters.
- `role:` field on AI nodes; `model:` fallback emits a deprecation warning.
- Budget meter wraps `AIExecutor`; live `$` printout per node.
- CLI: `roles list / describe / validate`; `--role-override role=model`;
  `--max-budget-dollars`.

**Ship criterion:** `dear-agent workflow lint --check-deprecated-models`
passes on all existing workflows after a one-time codemod. Switching
Opus 4.7 → Opus 5.0 is a one-line edit to `roles.yaml`.

### Phase 2 — DEAR hooks + HITL + exit_gate + outputs[]

**Goal:** the engine becomes substrate-grade — bounded permissions,
human-in-the-loop gates, declared outputs with durability tiers.

**Deliverables:**

- DEAR hook surface (`OnDefine`, `OnEnforce`, `OnAudit`, `OnResolve`).
- `awaiting_hitl` state; approver_role enforcement; timeout policies.
- Exit gate evaluator: 5 kinds (bash, regex_match, json_schema, test_cmd,
  confidence_score).
- Structured `outputs[]` with durability tiers
  (ephemeral / local_disk / git_committed / engram_indexed).
- Permission enforcer: fs_read / fs_write / network / tools / egress.
- CLI: `workflow approve / reject`.
- HITL backends: CLI (default), Discord (existing AGM bot), MCP.

**Ship criterion:** Wayfinder migrates one project end-to-end onto the
engine; Discord approval round-trip works; substrate score ≥ 4/5 on every
property.

### Phase 3 — FetchSource / AddSource

**Goal:** node outputs are addressable knowledge. `dear-agent search "topic"`
returns sources cited by previous research runs, joined to their work-items.

**Deliverables:**

- `pkg/source` adapter interface + SQLite + FTS5 default impl.
- MCP tools `FetchSource` / `AddSource`.
- Wire `outputs.durability=engram_indexed` to `AddSource`.
- CLI: `dear-agent search "query"`.

**Ship criterion:** `dear-agent search` returns results from the last 30
days of research outputs, joined to their work-item ids; FTS round-trip
P95 < 50 ms on 10K rows.

### Phase 4 — Migration + `workflow dev`

**Goal:** real workloads run on the engine; the inner-loop iteration
experience matches the synthesis's "10-minute walkthrough" target.

**Deliverables:**

- Codemod existing research pipeline workflow + Wayfinder phase definitions.
- `workflow dev` interactive shell with mock-by-default fixtures, hot-reload,
  and the `r / r --live / approve <node> / retry <node> / diff <node>` verbs.
- Documentation: `docs/workflow-engine.md` with the 10-minute walkthrough.

**Ship criterion:** new user goes `brew install` → useful workflow in ten
minutes; recorded fixtures make iteration sub-second; all existing workflows
have been migrated.

### Phase 5 — Adapters + visual inspector + `kind: spawn`

**Goal:** the engine is extensible. Plugin packaging keeps the core small.

**Deliverables (open-ended):**

- Obsidian adapter (single-user dual-write).
- llm-wiki adapter (markdown + git).
- OpenViking adapter (graph DB; future / enterprise).
- `kind: spawn` for emergent DAG growth.
- Visual run inspector (web UI reading SQLite).
- Plugin packaging.

---

## Role-based model mapping (design)

A central design decision in MVS: AI nodes declare a **role**, not a model.
A registry resolves the role to a primary/secondary/tertiary model tier
based on capability, capacity, and cost. Migrations become a one-line edit.

### Registry file

Resolution order (first found wins):

1. `$DEAR_AGENT_ROLES` (env var path)
2. `./.dear-agent/roles.yaml` (per-repo)
3. `~/.config/dear-agent/roles.yaml` (per-user)
4. Built-in defaults (compiled in)

### Schema

```yaml
version: 1
defaults:
  effort: high
  max_context: 200000

roles:
  research:
    description: "Long-context document analysis with citations"
    capabilities: [long_context, citations, web_search, structured_output]
    primary:
      model:        claude-opus-4-7
      effort:       max
      max_context:  1000000
      cost_per_mtok: { input: 15.00, output: 75.00 }
    secondary:
      model:        gemini-3.1-pro
      effort:       high
      max_context:  1000000
      cost_per_mtok: { input: 1.25, output: 10.00 }
    tertiary:
      model:        gpt-5.5-pro
      effort:       high

  implementer:
    description: "Code synthesis with tool use and patch application"
    capabilities: [tool_use, code_synthesis, structured_output]
    primary:
      model:  claude-sonnet-4-6
      effort: high

  reviewer:
    description: "Adversarial critique of artifacts produced by implementer"
    capabilities: [reasoning, structured_output]
    primary:
      model:  claude-opus-4-7
      effort: max
```

### Resolution algorithm

```
For each tier in [primary, secondary, tertiary]:
  if !satisfies(tier.capabilities, node.required_capabilities): skip
  if !capacity.has_budget(tier.model):                          skip
  if tier.min_cost(node) > node.budget.max_dollars:             skip
  if override := overrides.for(node, role):                     return override
  return tier
return NoModelAvailableError
```

Cost: O(tiers). Tiers ≤ 3. Constant per node.

### Override layering (highest precedence wins)

1. `--role-override role=model` (CLI flag, emits warning)
2. `model_override:` on a node (workflow YAML, emits warning)
3. `roles:` block in workflow YAML
4. `./.dear-agent/roles.yaml` (workspace)
5. `~/.config/dear-agent/roles.yaml` (user)
6. Built-in defaults

### Migration example

```diff
   research:
     primary:
-      model: claude-opus-4-7
+      model: claude-opus-5-0
```

Pre-migration audit:

```bash
dear-agent workflow lint --check-deprecated-models
# Lists every node with model_override or hardcoded model field
# pointing at a deprecated model.
```

---

## Node configuration schema (canonical)

Every field on a node maps onto a substrate property and onto a queryable
column in `audit_events` or `nodes`. Every field is optional except `id`,
`kind`, and the kind-specific body.

```yaml
schema_version: "1"
name:        deep-research
version:     0.3.0
description: "Three-angle deep research with synthesis"

inputs:
  - { name: question, required: true }
  - { name: corpus_path, required: false, default: "" }

env:
  WORKING_DIR: "{{ .Env.WORKING_DIR | default '.' }}"

defaults:
  permissions: { tools: [Read, Grep, FetchSource, AddSource] }
  budget:      { max_dollars: 5.00, max_wallclock: 30m }
  retry:       { max_attempts: 2, backoff: 30s, retry_on: [transient, rate_limit, context_overflow] }
  hitl:        { block_policy: never }

nodes:
  - id: research
    kind: ai
    role: research                              # MVS: replaces `model:`
    depends_on: [intake]
    when: ""                                    # branching predicate
    timeout: 30m

    # Phase 2: bounded permissions (substrate property)
    permissions:
      fs_read:   ["~/src/engram-research/**"]
      fs_write:  ["notes/{{ .RunID }}/**"]
      network:   ["anthropic.com", "github.com"]
      tools:     [Read, Grep, FetchSource, AddSource, WebSearch]
      egress_max_bytes: 5000000
      dynamic_paths: [".inputs.target_dir"]

    # Phase 2: human-in-the-loop as a first-class state
    hitl:
      block_policy: on_low_confidence            # never|always|on_low_confidence
      confidence_threshold: 0.7
      approver_role: research-lead
      timeout: 24h
      on_timeout: escalate                       # escalate|reject|approve

    # Phase 2: definition of done as data
    exit_gate:
      - { kind: schema_validation, target: outputs.report, schema: ./schemas/research-doc.json }
      - { kind: bash, cmd: "scripts/lint-research-doc.sh {{ .Outputs.report.path }}", success_exit: 0 }
      - { kind: confidence_score, target: outputs.report.frontmatter.confidence, min: 0.7 }
      # Other kinds: regex_match, test_cmd

    # Existing field, extended in MVS
    retry:
      max_attempts: 3
      backoff: 30s
      max_backoff: 10m
      retry_on: [transient, rate_limit, context_overflow]   # NEW: filter

    # MVS: cost is a permission
    budget:
      max_tokens: 300000
      max_dollars: 5.00
      max_wallclock: 30m
      on_overrun: escalate                       # escalate|fail|truncate

    # Phase 2: explicit context selection
    context_policy: selective                    # fresh|inherit|summarized|selective
    context_keys: [intake.brief, intake.corpus_summary]

    # Phase 2: declared outputs with durability tier
    outputs:
      report:
        path: "notes/{{ .RunID }}/report.md"
        content_type: text/markdown
        schema: ./schemas/research-doc.json
        durability: git_committed                # ephemeral|local_disk|git_committed|engram_indexed
      sources:
        path: "notes/{{ .RunID }}/sources.json"
        content_type: application/json
        schema: ./schemas/source-list.json
        durability: engram_indexed

    # AI body
    ai:
      prompt:
        template: ./prompts/research/research.md.tmpl
        engine: gotemplate
        variable_scope: [inputs, outputs, env, run]
      effort: max                                # role-tier hint; resolver may override
      max_tokens: 64000                          # output cap
```

### Substrate property → schema field map

| Substrate property | Schema fields | Phase |
|---|---|---|
| Durability | `outputs[].durability`, SQLite state | 0 (state) / 2 (outputs) |
| Ownership | `role`, `permissions.fs_*`, `hitl.approver_role` | 1 (role) / 2 (rest) |
| State machine | `pending → running → awaiting_hitl → succeeded/failed/skipped` | 0 |
| Audit | `audit_events` table; per-transition row | 0 |
| Bounded permissions | `permissions{fs,network,tools,egress}`, `budget` | 1 (budget) / 2 (rest) |

---

## State machine (canonical)

```
                  load + validate
                         │
                         ▼
                     [pending]
                  (DAG resolved)
                         │
            depends_on satisfied + when=true
                         │
                         ▼
                     [running] ─────── exit_gate fail ──────► [failed]
                         │                                       │
                hitl.block_policy fires                          │
                         │                                       │
                         ▼                                       │
              [awaiting_hitl] ────── reject / timeout ───────────┤
                         │                                       │
                       approve                                   │
                         │                                       │
                         ▼                                       │
                  exit_gate pass                                 │
                         │                                       │
                         ▼                                       │
                    [succeeded]                                  │
                                                                 │
   [pending] ── upstream failed ──► [skipped]                    │
   [pending] ── when=false ───────► [skipped]                    │
                                                                 ▼
                                                       on_failure resolvers
                                                                 │
                                                                 ▼
                                                            [resolved]
```

Every transition emits an `audit_events` row. Every retry adds a
`node_attempts` row.

---

## SQLite tables (canonical)

Full DDL: see [ADR-010 §5](docs/adrs/ADR-010-workflow-engine-architecture.md#5-storage-schema-canonical-sqlite).

| Table | Purpose |
|---|---|
| `workflows` | Cached workflow definitions, keyed by canonical hash |
| `runs` | One row per workflow execution — the work-item |
| `nodes` | Per-run, per-node aggregate state |
| `node_attempts` | One row per execution attempt (retries are visible) |
| `node_outputs` | Declared, durable artifacts |
| `audit_events` | One row per state transition (the substrate's audit log) |
| `approvals` | HITL records |

---

## Test of success (12-month checklist)

The engine is successful if all of these are true 12 months from now:

- [ ] An external developer can read a `research.yaml` PR diff and understand
      the change without docs.
- [ ] A model migration (e.g. Opus 4.7 → 5.0) is a one-line edit to
      `roles.yaml`, applied to every workflow.
- [ ] `dear-agent workflow status <run-id>` answers "what happened?" in under
      a second, with cost, retries, and HITL events visible.
- [ ] When a node fails, the CLI prints the three commands the user would
      run next.
- [ ] `dear-agent search "topic"` returns sources cited by previous research
      runs, joined to their work-items.
- [ ] Wayfinder runs as workflows, with HITL gates that page real humans
      on Discord.
- [ ] `dear-agent workflow dev` iterates on a prompt change in under one
      second using mocked fixtures.
- [ ] `SELECT * FROM audit_events WHERE run_id = ?` shows every transition,
      queryable, joined.
- [ ] Substrate score (durability/ownership/state-machine/audit/permissions)
      is ≥ 4/5 on every property by default.
- [ ] Existing v0.1 workflows still validate and run after a one-time codemod.

---

## How to pick up work

1. Read this file for the phase landscape.
2. Read [ADR-010](docs/adrs/ADR-010-workflow-engine-architecture.md) for
   the architectural decisions and their justifications.
3. Read [BACKLOG.md](docs/workflow-engine/BACKLOG.md) for individual
   tickets.
4. Pick a `pending` ticket in the current phase whose dependencies are
   `done`.
5. Mark it `in-flight` (with your session/branch) before starting work.
6. When you merge, mark it `done` (with the PR number) and update the
   phase status if appropriate.

If a decision is unclear, the synthesis doc on `engram-research/main` is
the source of truth. Quote a section ("§3.2") in the PR if you're
implementing against it.

---

## References

- [ADR-009 — Work Item as First-Class Substrate](docs/adrs/ADR-009-work-item-as-first-class-substrate.md)
- [ADR-010 — Workflow Engine Architecture](docs/adrs/ADR-010-workflow-engine-architecture.md)
- [BACKLOG — per-ticket tracking](docs/workflow-engine/BACKLOG.md)
- `~/src/engram-research/WORKFLOW-ENGINE-SYNTHESIS.md` (origin/main, 2026-05-02)
- `~/src/engram-research/WORKFLOW-ENGINE-RESEARCH-ENGINEERING.md` (origin/main, 2026-05-02)
