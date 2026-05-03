# ADR-010: Workflow Engine as Substrate-Quality Work-Item Layer

**Status**: Proposed
**Date**: 2026-05-02
**Context**: Workflow Engine research synthesis — see
`~/src/engram-research/WORKFLOW-ENGINE-SYNTHESIS.md` (and three sibling docs)
on `engram-research/main` as of 2026-05-02. Builds directly on
[ADR-009: Work Item as First-Class Substrate](ADR-009-work-item-as-first-class-substrate.md).
Implementation tracking: [ROADMAP.md](../../ROADMAP.md) and
[docs/workflow-engine/BACKLOG.md](../workflow-engine/BACKLOG.md).

This ADR captures the architectural decisions surfaced by the workflow-engine
research. Acceptance authorizes the work plan in `ROADMAP.md` to proceed.

---

## Context

`pkg/workflow` today is a small (~2700 LOC) YAML-driven DAG runner with
file-backed state, retries, parallelism, loop nodes, and an `AIExecutor`.
It is "Archon-shaped": it can run an agentic pipeline, but it does not
satisfy the substrate diagnostic from [ADR-009](ADR-009-work-item-as-first-class-substrate.md):

| Substrate property | Current state |
|---|---|
| Records | JSON file snapshot, not queryable, lossy |
| Ownership | Implicit; no role/assignee on a node |
| State machine | Implicit in code; transitions not auditable |
| Audit | Stdout logs only; no per-transition record |
| Bounded permissions | None — nodes can call any tool, write anywhere |

ADR-009 argued that dear-agent needs a first-class **work item** layer,
separate from the session layer. The workflow engine is the natural home
for it: every node-execution is a work-item record, every state transition
is an audit event. The engine and the work-item store are the same thing.

Three sibling research docs (`WORKFLOW-ENGINE-RESEARCH-CLAUDE.md`,
`-ENGINEERING.md`, `-ECOSYSTEM.md`) and a `-SYNTHESIS.md` on
`engram-research/main` independently arrived at the same component list and
roughly the same sequencing. That convergence is what this ADR is built on.

---

## Decision

Evolve `pkg/workflow` from "YAML DAG runner" into the **substrate-quality
work-item layer** the architecture review identified as missing. Specifically:

### D1. Three packages, one database

```
pkg/workflow/        (extended; additive only)
pkg/workflow/roles/  (new; role registry + resolver)
pkg/source/          (new; FetchSource / AddSource adapters)
```

One SQLite database (`runs.db`) backs the engine state. One pluggable adapter
backs source/output storage. One config file (`roles.yaml`) maps roles to
model tiers. Nothing else.

### D2. SQLite + WAL as the default state backend

Replace `FileState` JSON with `SQLiteState` (WAL mode). Tables:
`workflows, runs, nodes, node_attempts, node_outputs, audit_events,
approvals`. SQLite is chosen because:

- Single binary, no service to run, embedded.
- Built-in FTS5 covers full-text needs.
- Dolt-compatible schema preserves the option for users who want versioned
  SQL (existing AGM uses Dolt).
- A `.db` file is also the engine's debugger: `sqlite3 runs.db` mid-run
  answers "what is happening" with one query.

The `State` interface is preserved so a hosted Postgres adapter can ship
later if SQLite's ~10-concurrent-writer ceiling is hit.

### D3. Audit emission on every state transition

Every transition (`pending → running`, `running → awaiting_hitl`, etc.)
writes a row to `audit_events`. Sinks are pluggable; the SQLite default is
joined to the rest of the engine state, so `SELECT * FROM audit_events
WHERE run_id = ?` is the canonical "what happened" query. Additional sinks
(stdout, JSONL, Engram, OpenTelemetry) can be attached without changing
the runner.

### D4. Role-based model mapping (not hardcoded model strings)

AI nodes declare a `role:`, not a `model:`. A central registry resolves the
role to a primary/secondary/tertiary model tier based on capability,
capacity, and cost. Migrating the entire system from Opus 4.7 to Opus 5.0
becomes a one-line edit to `roles.yaml`. The existing `model:` field becomes
a back-compat fallback that emits a deprecation warning. Override layering
allows per-workflow, per-node, and per-CLI-invocation overrides.

Full schema and resolution algorithm: see `ROADMAP.md` "Role-based model
mapping (design)" and source doc `WORKFLOW-ENGINE-RESEARCH-ENGINEERING.md`
§3 on `engram-research/main`.

### D5. Bounded permissions and budget as data on the node

Each node declares:

- `permissions` (fs_read / fs_write / network / tools / egress allowlists)
- `budget` (max_tokens / max_dollars / max_wallclock + on_overrun policy)

Permissions are enforced by a thin wrapper that asks the harness "can this
tool be used here?" — the harness remains the enforcer; the engine
declares the policy. Budget is enforced per-node and per-run; the meter
wraps `AIExecutor` and emits live `$` printouts to the CLI.

### D6. HITL as a first-class state, not a callback

A node can be in `awaiting_hitl`. The state is persisted, queryable, and
addressable from outside the engine (CLI, Discord bot, MCP client). The
node declares `approver_role`, `timeout`, and `on_timeout` policy. The
existing AGM Discord bot becomes one of several pluggable HITL backends.

### D7. Exit gates as data, not code

Definition-of-done becomes a YAML list of gate evaluations. Five kinds in
v1: `bash`, `regex_match`, `json_schema`, `test_cmd`, `confidence_score`.
Gates evaluate in order and short-circuit. A failing gate transitions the
node to `failed`; on_failure resolvers fire from there.

### D8. Structured outputs with durability tier per artifact

`outputs:` becomes a map of `key → { path, content_type, schema,
durability }`. The `durability` tier is one of `ephemeral`, `local_disk`,
`git_committed`, `engram_indexed`. The engine writes/commits/indexes per
the declared tier. The runner refuses to mark a node `succeeded` until
all declared outputs exist.

### D9. FetchSource / AddSource as the canonical knowledge surface

Two MCP tools (`FetchSource` / `AddSource`) and an adapter interface
(`pkg/source.Adapter`) become the canonical way for nodes to read and
write durable knowledge. The default adapter is SQLite + FTS5; Obsidian,
llm-wiki, and (future) OpenViking can plug in. Outputs declared with
`durability=engram_indexed` are written through `AddSource` and become
searchable via `dear-agent search`.

### D10. No determinism contract; replay is an audit feature

The engine is not Temporal. We do not promise deterministic replay. What
we promise is that the audit log is complete and queryable enough to
reconstruct what happened. Replay mode (`workflow run --resume <run-id>`)
re-runs from the snapshot; reproducibility is a function of the LLM and
the inputs, not of the engine. This keeps the implementation small and
honest.

### D11. YAML stays the authoring format; no visual editor in v1

YAML round-trips through git. Prompts move out of the YAML into
`prompts/<workflow>/<node>.md.tmpl` files. A 1000-line YAML is a smell,
not the norm. A future renderer over YAML is acceptable; an authoring UI
is not in scope.

### D12. MVS = D2 + D3 + D4 + D5(budget only) ships first

The Minimum Viable Substrate is the smallest cut that satisfies all five
substrate properties by default. Defined precisely in `ROADMAP.md`. The
six-phase release plan in that doc operationalizes this decision.

---

## Architecture diagram

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
   │  pkg/workflow                                                  │
   │  ┌─────────────────────────────────────────────────────────┐   │
   │  │ Runner      (existing, extended)                         │   │
   │  │ Validator   (existing, extended)                         │   │
   │  │ DEAR hooks  (NEW: OnDefine/Enforce/Audit/Resolve)        │   │
   │  │ Permission enforcer  (NEW)                               │   │
   │  │ Budget meter  (NEW)                                      │   │
   │  │ Exit gates  (NEW)                                        │   │
   │  │ HITL backend  (NEW)                                      │   │
   │  │ Role resolver  (NEW)                                     │   │
   │  └─────────────────────────────────────────────────────────┘   │
   │                                                                │
   │  pkg/workflow state:  SQLite schema                            │
   │   - workflows                                                  │
   │   - runs                                                       │
   │   - nodes (per-run, per-node)                                  │
   │   - node_attempts                                              │
   │   - node_outputs                                               │
   │   - audit_events  ← every state transition                     │
   │   - approvals                                                  │
   │                                                                │
   │  Surfaces:  CLI, MCP tools, Go SDK                             │
   └─────────┬───────────────────────────────────┬──────────────────┘
             │ outputs.durability=engram_indexed │ context query
             │ via FetchSource / AddSource       │
             ▼                                   ▼
   ┌────────────────────────────────┐  ┌──────────────────────────┐
   │         pkg/source             │  │   Role registry          │
   │  ┌──────────────────────────┐  │  │   ~/.config/.../         │
   │  │  SQLite adapter (default)│  │  │   roles.yaml             │
   │  │  Obsidian adapter (opt)  │  │  │                          │
   │  │  llm-wiki adapter (opt)  │  │  │                          │
   │  │  OpenViking (future)     │  │  │                          │
   │  └──────────────────────────┘  │  │                          │
   └────────────────────────────────┘  └──────────────────────────┘
                  ▲
                  │  reads via FetchSource
                  │
   ┌──────────────┴──────────────────────────────────────────────┐
   │                        Engram                                │
   │     knowledge state: cues, memory entries, beads             │
   └──────────────────────────────────────────────────────────────┘
```

---

## What changes

| Layer | Change |
|---|---|
| `pkg/workflow/types.go` | Additive: `role`, `permissions`, `budget`, `exit_gate`, `hitl`, `context_policy`, structured `outputs[]`. Existing `RetryPolicy` extended with `retry_on`. New `kind: spawn` (Phase 5). |
| `pkg/workflow/state_*.go` | `SQLiteState` becomes default. `FileState` deprecated, kept for tiny embedded use. New tables per §5 below. |
| `pkg/workflow/runner.go` | Emits `audit_events` per state transition. Wraps `AIExecutor` with budget meter. Calls into permission enforcer. Calls into role resolver before each AI node. |
| `pkg/workflow/roles/` (new) | Role registry + tier resolver. `~/.config/dear-agent/roles.yaml`. |
| `pkg/source/` (new) | Adapter interface + SQLite default impl. |
| `cmd/dear-agent-mcp/` | New tools: `workflow_*` (5 tools), `FetchSource`, `AddSource`. |
| `cmd/workflow-*` | New CLI commands: `status`, `list`, `cancel`, `logs`, `approve`, `lint`, `migrate`, `dev`. |

## What does **not** change

- The YAML shape (additive only). Existing workflows pass `lint` after a
  one-time codemod adds `schema_version: "1"`.
- The DAG topology mechanics (`depends_on`, cycle detection, topological sort).
- The `LoopNode` semantics (sequential `until:` and parallel `max_iters` modes).
- The harness / sandbox boundary. The engine asks the harness "can this tool be
  used here?"; the harness enforces.
- The `cmd/workflow-run` binary stays as the primary entry point (extended).

---

## 5. Storage schema (canonical SQLite)

```sql
-- Workflow definitions, cached by canonical YAML hash.
CREATE TABLE workflows (
  workflow_id    TEXT PRIMARY KEY,           -- hash of canonicalized YAML
  name           TEXT NOT NULL,
  version        TEXT NOT NULL,
  yaml_canonical TEXT NOT NULL,
  registered_at  TIMESTAMP NOT NULL,
  UNIQUE(name, version)
);
CREATE INDEX idx_workflows_name ON workflows(name);

-- One row per run. The work-item.
CREATE TABLE runs (
  run_id          TEXT PRIMARY KEY,           -- UUID
  workflow_id     TEXT NOT NULL REFERENCES workflows(workflow_id),
  state           TEXT NOT NULL CHECK (state IN
                  ('pending','running','awaiting_hitl','succeeded','failed','cancelled')),
  inputs_json     TEXT NOT NULL,
  started_at      TIMESTAMP NOT NULL,
  finished_at     TIMESTAMP,
  total_tokens    INTEGER NOT NULL DEFAULT 0,
  total_dollars   REAL    NOT NULL DEFAULT 0,
  error           TEXT,
  trigger         TEXT,                       -- cli|mcp|sdk|cron|trigger
  triggered_by    TEXT
);
CREATE INDEX idx_runs_state         ON runs(state);
CREATE INDEX idx_runs_started_at    ON runs(started_at);
CREATE INDEX idx_runs_workflow_id   ON runs(workflow_id);

-- Per-run, per-node aggregate state.
CREATE TABLE nodes (
  run_id        TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
  node_id       TEXT NOT NULL,
  state         TEXT NOT NULL CHECK (state IN
                ('pending','running','awaiting_hitl','succeeded','failed','skipped')),
  attempts      INTEGER NOT NULL DEFAULT 0,
  role_used     TEXT,
  model_used    TEXT,
  tokens_used   INTEGER NOT NULL DEFAULT 0,
  dollars_spent REAL    NOT NULL DEFAULT 0,
  started_at    TIMESTAMP,
  finished_at   TIMESTAMP,
  error         TEXT,
  PRIMARY KEY (run_id, node_id)
);
CREATE INDEX idx_nodes_state ON nodes(state);

-- Per-attempt detail (one row per execution attempt; retries are visible).
CREATE TABLE node_attempts (
  attempt_id    TEXT PRIMARY KEY,             -- UUID
  run_id        TEXT NOT NULL,
  node_id       TEXT NOT NULL,
  attempt_no    INTEGER NOT NULL,
  state         TEXT NOT NULL,
  model_used    TEXT,
  prompt_hash   TEXT,
  response_hash TEXT,
  tokens_used   INTEGER NOT NULL DEFAULT 0,
  dollars_spent REAL    NOT NULL DEFAULT 0,
  started_at    TIMESTAMP NOT NULL,
  finished_at   TIMESTAMP,
  error_class   TEXT,
  error_message TEXT,
  FOREIGN KEY (run_id, node_id) REFERENCES nodes(run_id, node_id) ON DELETE CASCADE,
  UNIQUE (run_id, node_id, attempt_no)
);
CREATE INDEX idx_node_attempts_run_node ON node_attempts(run_id, node_id);

-- Declared, durable artifacts.
CREATE TABLE node_outputs (
  run_id        TEXT NOT NULL,
  node_id       TEXT NOT NULL,
  output_key    TEXT NOT NULL,                -- e.g. "report"
  path          TEXT NOT NULL,                -- resolved path / URI
  content_type  TEXT,
  durability    TEXT NOT NULL CHECK (durability IN
                ('ephemeral','local_disk','git_committed','engram_indexed')),
  size_bytes    INTEGER,
  hash          TEXT,
  indexed_at    TIMESTAMP,
  PRIMARY KEY (run_id, node_id, output_key),
  FOREIGN KEY (run_id, node_id) REFERENCES nodes(run_id, node_id) ON DELETE CASCADE
);

-- One row per state transition. The substrate's audit log.
CREATE TABLE audit_events (
  event_id    TEXT PRIMARY KEY,              -- UUID
  run_id      TEXT NOT NULL,
  node_id     TEXT,                           -- nullable for run-level events
  attempt_no  INTEGER,
  from_state  TEXT,
  to_state    TEXT NOT NULL,
  reason      TEXT,
  actor       TEXT NOT NULL,                  -- system|role:research|human:vbonnet|mcp:client-id
  occurred_at TIMESTAMP NOT NULL,
  payload_json TEXT
);
CREATE INDEX idx_audit_events_run     ON audit_events(run_id, occurred_at);
CREATE INDEX idx_audit_events_actor   ON audit_events(actor);
CREATE INDEX idx_audit_events_to_state ON audit_events(to_state);

-- HITL records.
CREATE TABLE approvals (
  approval_id   TEXT PRIMARY KEY,
  run_id        TEXT NOT NULL,
  node_id       TEXT NOT NULL,
  requested_at  TIMESTAMP NOT NULL,
  resolved_at   TIMESTAMP,
  decision      TEXT CHECK (decision IN ('approve','reject','timeout')),
  approver      TEXT,
  approver_role TEXT,
  reason        TEXT,
  FOREIGN KEY (run_id, node_id) REFERENCES nodes(run_id, node_id) ON DELETE CASCADE
);
CREATE INDEX idx_approvals_pending ON approvals(run_id, node_id) WHERE resolved_at IS NULL;
```

### Retention policy

| Table | Retention | Mechanism |
|---|---|---|
| `runs` | indefinite | manual purge command; default keep all |
| `nodes` | follows `runs` | cascade |
| `node_attempts` | 90 days for succeeded; indefinite for failed | nightly job |
| `node_outputs` | follows `runs` | cascade; durability tier handles its own retention |
| `audit_events` | 1 year for `succeeded` runs; indefinite for `failed`/`cancelled` | nightly job |
| `approvals` | follows `runs` | cascade |

---

## 6. Performance targets

| Operation | Target |
|---|---|
| Read run status (with all nodes) for 100-node DAG | P95 < 5 ms |
| Read run status for 10K-node DAG | P95 < 50 ms |
| Append audit event | P95 < 1 ms |
| List 50 most recent runs | P95 < 10 ms |
| Concurrent runs writing simultaneously (10x) | no lock contention; SQLite WAL mode required |

Validated by `runner_perf_test.go` (new file, Phase 0).

---

## Consequences

### Positive

- **Substrate score jumps from 1/5 to 5/5** on the workflow layer. Every
  node-execution is a queryable record with explicit ownership, a defined
  state machine, an audit trail, and bounded permissions.
- **Model migrations become trivial.** One line in `roles.yaml` switches
  every workflow from one model to another. The role abstraction is the
  single biggest force-multiplier in the design.
- **The engine is its own debugger.** `sqlite3 runs.db` answers any "what
  happened?" question with one query. No log-file archaeology.
- **Existing AGM components opt in additively.** Nothing in AGM, Engram,
  Wayfinder, or Beads must change. They become audit subscribers, source
  adapters, or HITL backends. Migration is per-component and per-week, not
  big-bang.
- **The path to Wayfinder-as-workflow is unblocked.** Phase = node, gate =
  exit_gate, review = HITL. One Wayfinder project migrated end-to-end is
  the Phase 2 ship criterion.
- **The development inner loop has a story.** `workflow dev` with mock
  fixtures is the unfilled niche the ecosystem doc identified.

### Negative / costs

- **2700 LOC of `pkg/workflow` need careful extension.** Most changes are
  additive, but `state_file.go → state_sqlite.go` is a real swap with a
  migration path. ~4 weeks of focused work for Phase 0 alone.
- **Two new packages (`pkg/workflow/roles`, `pkg/source`) are surface
  area we have to maintain.** The role registry and adapter interface
  both have to be designed well enough to not need v2 in 12 months.
- **YAML schema grows.** Additive, but a node spec at full power is now a
  significant chunk of YAML. The `defaults:` block and prompt files are
  the mitigations; `workflow lint` enforces them.
- **SQLite single-writer ceiling.** ~10 concurrent writers is the
  practical limit. Beyond that, a hosted Postgres adapter is needed. This
  is bet #3 in the synthesis (medium stakes, hedged by the `State`
  interface).
- **HITL via Discord/CLI may not be enough for some teams.** Slack /
  Linear / custom UIs are downstream work. Hedged by the pluggable HITL
  backend.
- **No deterministic replay is a deliberate non-goal.** Teams who want
  Temporal's guarantees should use Temporal. We are choosing inspectability
  over determinism.

### Neutral

- **Naming collision with the existing 9-phase Wayfinder methodology.**
  The engine has 6 implementation phases; Wayfinder has 9 SDLC phases.
  These are unrelated. Documentation always disambiguates ("engine
  Phase 0" vs "Wayfinder CHARTER phase").
- **No visual editor in v1 is a deliberate constraint, not a deferral.**
  We may add a *renderer* over YAML if demanded; we will not add an
  *authoring* UI.

---

## Bets, ranked by stakes

The synthesis enumerates these. We accept all of them by accepting this ADR.

**High stakes (we lose if these are wrong):**

1. **YAML is the right authoring format.** Hedged by prompt files + role
   registry + workflow defaults; a 1000-line YAML is a smell.
2. **Substrate framing produces a better product than incremental
   "add-features-to-Archon".** Hedged by sensible defaults — users who
   don't reach for permissions/audit don't pay for them.
3. **SQLite scales for our user base.** Hedged by the `State` interface
   for a future Postgres adapter.

**Medium stakes:**

4. **Roles are the right abstraction.** Hedged by `model_override` escape
   hatch.
5. **HITL via Discord/CLI is enough.** Hedged by the pluggable backend.
6. **Mock-by-default `workflow dev` is the right inner loop.** Hedged by
   `--live` being one keystroke away.

**Low stakes:** the exact list of exit_gate kinds; the exact retention
policy for audit_events; whether OpenViking ever ships.

---

## Alternatives considered

| Alternative | Why rejected |
|---|---|
| **Build on Temporal.** | Heavyweight (separate service); imposes determinism contract we don't want; obscures the substrate properties behind a worker SDK; locks us in. Useful for teams whose problem is "long-running orchestration" — not ours. |
| **Build on LangGraph.** | Python-only; in-memory by default; the substrate properties are a user-built layer, not provided. We would end up rebuilding most of this ADR inside LangGraph. |
| **Stay on Archon-shaped `pkg/workflow` and add fields incrementally.** | The structural choices (SQLite, audit_events, roles) need to land together to be coherent. Incremental adds without the substrate framing produce a bag of features, not a layer. |
| **Visual workflow editor.** | YAML round-trips through git; a UI complicates review. A renderer (read-only) is acceptable later; an authoring UI is not in scope. |
| **Postgres as default state backend.** | Service-to-run cost. SQLite + the `State` interface gets us 95% of users with zero ops burden. Postgres adapter ships if/when needed. |
| **Code-as-workflow (Go DSL).** | Loses the `workflow lint` / `workflow dev` story; harder to read in a PR diff; requires a Go toolchain to author. We may add a Go SDK that produces YAML, but YAML stays canonical. |
| **Make Wayfinder the substrate.** | Wayfinder is a 9-phase *methodology* (CHARTER/PROBLEM/RESEARCH/...). It runs *on* the engine; it isn't the engine. The two solve different problems. |

---

## Open questions

These are deferred to later ADRs or later phase reviews:

1. **Migration path for existing JSON snapshots.** We commit to providing
   `workflow migrate` (Phase 4 ticket 4.3). The exact UX of round-tripping
   a half-finished JSON run into SQLite is undecided.
2. **Per-tenant isolation.** The current design assumes one user / one
   `runs.db`. Multi-tenant deployments need a row-level tenant column or
   per-tenant DBs. Punted to post-MVS.
3. **Cost-per-mtok refresh cadence.** `roles.yaml` carries cost numbers
   that drift with vendor pricing. We may want a sidecar refresher; for
   now, the operator updates `roles.yaml` manually.
4. **HITL via GitHub PRs.** Several patterns suggest "open a PR; merge =
   approve" as a useful HITL backend. Not in v1.
5. **Schema evolution policy.** When we add a column to `audit_events`, do
   we run `ALTER` on existing DBs, or version the schema and migrate? The
   "Dolt-compatible" constraint biases us to additive-only, but the policy
   needs to be written down before Phase 2.

---

## Implementation plan

The phased roadmap, ticket-level detail, parallelism, and ship criteria
are in [ROADMAP.md](../../ROADMAP.md) and
[docs/workflow-engine/BACKLOG.md](../workflow-engine/BACKLOG.md).

In summary:

- **Phase 0 (4 weeks):** SQLite + audit_events. MVS pt. 1.
- **Phase 1 (3 weeks):** Roles + budget. MVS pt. 2.
- **Phase 2 (4 weeks):** DEAR hooks + HITL + exit_gate + outputs[].
- **Phase 3 (2 weeks):** FetchSource / AddSource. Parallelizable with 1+2.
- **Phase 4 (4 weeks):** Migration + `workflow dev`.
- **Phase 5 (open-ended):** Adapters, visual inspector, `kind: spawn`.

Phases 0+1 together = MVS, the first user-visible release.

---

## Status

This ADR is **Proposed**, not Accepted.

Acceptance authorizes the work plan in `ROADMAP.md` to proceed and commits
the project to the architectural decisions D1–D12 above. The decisions are
binding for the duration of MVS (Phases 0+1); revisiting them requires a
follow-up ADR that quotes the relevant decision number.

Acceptance does NOT pre-authorize:

- Schema changes to existing AGM, Engram, or Wayfinder packages (still
  require their own review).
- New external dependencies beyond `modernc.org/sqlite` (already in tree)
  and any fixtures needed for tests.
- Breaking changes to `pkg/workflow`'s YAML schema. All MVS additions are
  additive; back-compat is enforced by `workflow lint`.

---

## References

- [ADR-009 — Work Item as First-Class Substrate](ADR-009-work-item-as-first-class-substrate.md)
- [ROADMAP.md — phased delivery](../../ROADMAP.md)
- [docs/workflow-engine/BACKLOG.md — per-ticket tracking](../workflow-engine/BACKLOG.md)
- `~/src/engram-research/WORKFLOW-ENGINE-SYNTHESIS.md` (origin/main, 2026-05-02)
- `~/src/engram-research/WORKFLOW-ENGINE-RESEARCH-CLAUDE.md` (origin/main, 2026-05-02)
- `~/src/engram-research/WORKFLOW-ENGINE-RESEARCH-ENGINEERING.md` (origin/main, 2026-05-02)
- `~/src/engram-research/WORKFLOW-ENGINE-RESEARCH-ECOSYSTEM.md` (origin/main, 2026-05-02)
- `~/src/engram-research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md` (origin/main, prior)
