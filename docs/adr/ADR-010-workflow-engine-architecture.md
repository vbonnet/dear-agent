# ADR-010: Workflow Engine as Substrate-Quality Work-Item Layer

**Status**: Proposed (2026-05-02)

`pkg/workflow` is a YAML-driven DAG runner with JSON file state. It can
run agentic pipelines but fails every property of the work-item substrate
from [ADR-009](ADR-009-work-item-as-first-class-substrate.md): records are
lossy and unqueryable, ownership is implicit, transitions are not
auditable, and nodes can call any tool. ADR-009 argued for a first-class
work-item layer; the workflow engine is the natural home. Every
node-execution is a work-item record; every state transition is an audit
event.

Evolve `pkg/workflow` into the substrate-quality layer. Twelve binding
decisions for the duration of MVS (Phases 0+1); revising any of them
requires a follow-up ADR quoting the decision number.

- **D1. Three packages, one database.** `pkg/workflow/` (extended),
  `pkg/workflow/roles/` (new), `pkg/source/` (new). One SQLite `runs.db`,
  one pluggable adapter for source/output storage, one `roles.yaml`.
- **D2. SQLite + WAL as the default state backend.** Tables: `workflows`,
  `runs`, `nodes`, `node_attempts`, `node_outputs`, `audit_events`,
  `approvals`. Embedded, no service to run, FTS5 covers full-text, and
  `sqlite3 runs.db` mid-run answers "what is happening" with one query.
  The `State` interface is preserved so a Postgres adapter ships later if
  the ~10-concurrent-writer ceiling bites.
- **D3. Audit emission on every state transition.** A row to
  `audit_events` per `pending → running`, `running → awaiting_hitl`, etc.
  The SQLite default joins the rest of the engine state; stdout / JSONL /
  Engram / OTel sinks attach without changing the runner.
- **D4. Roles, not models.** AI nodes declare `role:`. A central registry
  resolves to a primary/secondary/tertiary tier. Migrating Opus 4.7 → 5.0
  becomes one line of `roles.yaml`. `model:` survives as a back-compat
  override; per-workflow / per-node / per-CLI overrides layer cleanly.
- **D5. Bounded permissions and budget as data on the node.** Each node
  declares `permissions` (fs/network/tools/egress allowlists) and `budget`
  (max_tokens / max_dollars / max_wallclock + on_overrun policy). The
  harness enforces; the engine declares.
- **D6. HITL is a first-class state, not a callback.** `awaiting_hitl` is
  persisted, queryable, and addressable from outside (CLI, Discord bot,
  MCP client). Backends are pluggable.
- **D7. Exit gates as data, not code.** YAML list of gate evaluations:
  `bash`, `regex_match`, `json_schema`, `test_cmd`, `confidence_score`.
  Evaluate in order, short-circuit, fail to `failed` → `on_failure`.
- **D8. Structured outputs with a per-artifact durability tier.**
  `outputs:` is `key → { path, content_type, schema, durability }` where
  durability is `ephemeral | local_disk | git_committed | engram_indexed`.
  The runner refuses to mark a node succeeded until declared outputs
  exist.
- **D9. FetchSource / AddSource as the canonical knowledge surface.** Two
  MCP tools plus a `pkg/source.Adapter` interface. Default adapter is
  SQLite + FTS5; Obsidian and llm-wiki plug in.
- **D10. No determinism contract; replay is an audit feature.** The
  engine is not Temporal. We promise the audit log is complete enough to
  reconstruct what happened; replay reruns from the snapshot.
  Reproducibility is a function of the LLM, not the engine.
- **D11. YAML stays the authoring format; no visual editor in v1.**
  Prompts move into `prompts/<workflow>/<node>.md.tmpl` files. A
  1000-line YAML is a smell. A read-only renderer is acceptable later;
  an authoring UI is not in scope.
- **D12. MVS = D2 + D3 + D4 + D5(budget) ships first.** The Minimum
  Viable Substrate is the smallest cut that satisfies all five substrate
  properties by default. Completed delivery is visible in Git history and
  current follow-up work is tracked in Beads.

### Alternatives rejected

- **Build on Temporal.** Heavyweight; imposes a determinism contract we
  do not want; obscures substrate properties behind a worker SDK.
- **Build on LangGraph.** Python-only; in-memory by default; substrate
  properties are user-built.
- **Add fields incrementally to the Archon-shaped runner.** SQLite,
  `audit_events`, and roles need to land together to be coherent.
- **Postgres as default.** Service-to-run cost for ~95% of users.
- **Code-as-workflow (Go DSL).** Loses `workflow lint` / `workflow dev`;
  harder to read in a PR; requires a Go toolchain to author.
- **Make Wayfinder the substrate.** Wayfinder is a 9-phase *methodology*;
  it runs on the engine, it is not the engine.

### Bets ranked by stakes

High: YAML is the right authoring format; substrate framing beats
incremental Archon; SQLite scales for our user base. Medium: roles are
the right abstraction; Discord/CLI HITL is enough; mock-by-default
`workflow dev` is the right inner loop. Hedges: prompt files + lint;
`State` interface for a Postgres adapter; `model_override` escape hatch;
pluggable HITL backend.

> **Terminology.** `OnDefine/OnEnforce/OnAudit/OnResolve` are the
> workflow-engine *workflow lifecycle hooks*, not the canonical *process*
> DEAR retrospective (Define / **Execute** / Audit / **Retro**). See
> [ADR-035](ADR-035-dear-terminology-disambiguation.md). No code rename
> implied.

Storage schema, retention policy, and performance targets are in the
[workflow engine guide](../workflow-engine.md); source synthesis lives on
`engram-research/main` (`WORKFLOW-ENGINE-SYNTHESIS.md` and three sibling
research docs, 2026-05-02).
