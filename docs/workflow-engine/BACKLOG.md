# Workflow Engine — Backlog

**Status:** Active
**Last updated:** 2026-05-03 (cross-phase X.5 + X.6 closed)
**Source of truth for:** individual tickets within each phase. Phase-level
status and architecture decisions live in
[ROADMAP.md](../../ROADMAP.md) and
[ADR-010](../adrs/ADR-010-workflow-engine-architecture.md). Read both before
picking up a ticket.

This file is the per-ticket backlog. One row per ticket. Update the `Status`
column inline as work progresses.

---

## How to use this file

**Pick a ticket:** find the lowest-numbered `pending` ticket in the active
phase whose `Dep` column is satisfied (all listed tickets are `done`). If
no ticket in the active phase qualifies, look at parallelizable phases per
the ROADMAP parallelism map.

**Claim a ticket:** edit its `Status` from `pending` → `in-flight (<branch>)`
in a small commit before starting work. This prevents two sessions from
grabbing the same ticket.

**Complete a ticket:** when the PR merges, edit `Status` to
`done (#<PR>)`. If the ticket revealed a follow-up that wasn't in scope,
add a new ticket at the end of its phase rather than expanding the
original.

**Sizes:** S ≤ 3 days, M ≤ 2 weeks, L ≤ 4 weeks. If a ticket is sliding
past its size, split it before continuing.

**Status values:** `pending`, `in-flight (<branch>)`, `blocked (<reason>)`,
`done (#<PR>)`.

---

## Phase 0 — SQLite + audit_events (MVS pt. 1)

**Goal:** every node-execution is a queryable row, every state transition
is an audit event. Existing workflows run unchanged.
**Phase status:** `done (#38)`
**Estimated:** 4 weeks

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 0.1 | Add `SQLiteState` implementing existing `State` interface | `pkg/workflow/state_sqlite.go`, `state_sqlite_test.go` | Pass existing `state_test.go` semantics; new `runner_perf_test.go` passes targets in ADR-010 §6 | — | M | `done (#38)` |
| 0.2 | Migrate `Snapshot` representation to relational schema | `pkg/workflow/types.go`, `state_sqlite.go` | All existing snapshot fields representable; backward-compatible JSON dump retained for `workflow migrate` | 0.1 | S | `done (#38)` |
| 0.3 | Add `runs` + `nodes` + `node_attempts` tables; runner writes per-attempt rows | `pkg/workflow/runner.go`, `state_sqlite.go`, schema migration | After a 5-node run with one retry, `SELECT * FROM nodes` shows 5 rows and `SELECT * FROM node_attempts` shows 6 rows | 0.2 | M | `done (#38)` |
| 0.4 | Add `audit_events` table + `AuditSink` interface; runner emits transitions | `pkg/workflow/audit.go`, `runner.go` | Every state transition produces an `audit_events` row; replayable from sink | 0.3 | M | `done (#38)` |
| 0.5 | CLI `dear-agent workflow status <run-id>` reading from SQLite | `cmd/workflow-status/main.go` | Output matches the spec in `ROADMAP.md`; `--json` and `--watch` flags work | 0.4 | S | `done (#38)` |
| 0.6 | CLI `dear-agent workflow list / cancel / logs` | `cmd/workflow-list/`, `cmd/workflow-cancel/`, `cmd/workflow-logs/` | All three commands tested against a real SQLite DB; respect `--state=` filter | 0.5 | M | `done (#38)` |

**Phase 0 ship criterion:** existing workflows run unchanged. `SELECT *
FROM audit_events WHERE run_id = ?` returns every transition for the run.
Performance: read run status for 100-node DAG P95 < 5 ms; append audit
event P95 < 1 ms (validated by `runner_perf_test.go`).

---

## Phase 1 — Roles + budget (MVS pt. 2)

**Goal:** AI nodes declare `role:` not `model:`. A central registry
resolves the role to a model tier. Budgets are enforced per-node and
per-run.
**Phase status:** `done`
**Estimated:** 3 weeks
**Depends on:** Phase 0

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 1.1 | Schema additions: `role`, `permissions`, `budget`, `exit_gate`, `hitl`, `context_policy`, `outputs[]` | `pkg/workflow/types.go`, `load.go`, `load_test.go` | YAML round-trips; `Validate` accepts/rejects per ADR-010 §D; existing workflows still pass | 0.* | M | `done` |
| 1.2 | Role registry + resolver | `pkg/workflow/roles/registry.go`, `roles/resolver.go`, `roles/registry_test.go` | Resolves correctly for primary/secondary/tertiary; capacity, cost, capability filters per ROADMAP "Resolution algorithm" | 1.1 | M | `done` |
| 1.3 | Budget enforcement at `AIExecutor` wrapper | `pkg/workflow/budget.go`, `budget_test.go` | Run hitting ceiling triggers `on_overrun` policy (escalate/fail/truncate); live `$` printout in CLI | 1.2 | S | `done` |
| 1.4 | Permission enforcer interface; bash + ai check tool/path allowlists | `pkg/workflow/permissions.go`, `permissions_test.go` | Rejected tool call produces audit row + node failure; allowlist semantics match ADR-010 §D5 | 1.1 | M | `done` |
| 1.5 | Exit-gate evaluator (5 kinds: bash, regex_match, json_schema, test_cmd, confidence_score) | `pkg/workflow/exit_gate.go`, `exit_gate_test.go` | Each kind has unit tests; gate failure short-circuits and transitions node to `failed`; gates evaluated in declared order | 1.1 | M | `done` |
| 1.6 | `outputs[]` map-shape + path resolution + durability tier writer | `pkg/workflow/outputs.go`, `outputs_test.go` | Files materialize at declared paths; `git_committed` writes a commit; node refuses `succeeded` if a declared output is missing | 1.1 | M | `done` |
| 1.7 | `workflow lint` + `workflow roles` commands | `cmd/workflow-lint/main.go`, `cmd/workflow-roles/main.go` | `--check-deprecated-models` lists every node with hardcoded `model:` or `model_override:` pointing at a deprecated model; `workflow-roles list/describe/validate` matches ROADMAP "Role-based model mapping" | 1.2 | S | `done` |

**Phase 1 ship criterion:** `dear-agent workflow lint
--check-deprecated-models` passes on all existing workflows after a
one-time codemod. Switching Opus 4.7 → Opus 5.0 is a one-line edit to
`roles.yaml` and produces a queryable audit row showing the new model
in use.

---

## Phase 2 — DEAR hooks + HITL + exit_gate + outputs[]

**Goal:** the engine becomes substrate-grade — bounded permissions,
human-in-the-loop gates, declared outputs with durability tiers.
**Phase status:** `done (#40)`
**Estimated:** 4 weeks
**Depends on:** Phase 1

> Note: tickets 1.4 (permissions), 1.5 (exit_gate), and 1.6 (outputs[])
> land in Phase 1 schema-wise. Phase 2 wires them into the runner's
> hook surface and adds HITL.

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 2.1 | DEAR hook surface: `OnDefine`, `OnEnforce`, `OnAudit`, `OnResolve` | `pkg/workflow/hooks.go`, `hooks_test.go` | Each hook called with documented payload; hook errors surfaced in audit_events | 1.* | S | `done (#40)` |
| 2.2 | HITL: `awaiting_hitl` state, approver_role check, timeout policy | `pkg/workflow/hitl.go`, `runner_hitl.go` | Block → approve via CLI → resume; timeout fires `on_timeout`; rejection transitions node to `failed` | 1.1 | M | `done (#40)` |
| 2.3 | `dear-agent workflow approve / reject` CLI | `cmd/workflow-approve/main.go` | Round-trip with HITL backend; `--as <role>` enforces approver_role match; `--reason` audit-logged | 2.2 | S | `done (#40)` |
| 2.4 | Audit subscribers: stdout, JSONL file, Engram, OpenTelemetry | `pkg/workflow/audit_stdout.go`, `audit_jsonl.go`, `audit_engram.go`, `audit_otel.go` | Each sink has tests; sinks composable; failure of one sink doesn't break the run | 2.1 | M | `done (#40)` |
| 2.5 | MCP server with 5 workflow tools | `cmd/dear-agent-mcp/workflow.go` | Tools: `workflow_run / status / approve / reject / cancel`. All callable from a vanilla MCP client | 2.3 | M | `done (#40)` |
| 2.6 | Discord HITL backend (extends existing AGM bot) | `pkg/hitl/discord/backend.go` | Bot reads `audit_events` for new HITL rows; renders summary; reply-to-approve writes to `approvals` table | 2.4 | M | `done (#40)` |

**Phase 2 ship criterion:** Wayfinder migrates one project end-to-end
onto the engine. Discord approval round-trip works. Substrate score
≥ 4/5 on every property.

---

## Phase 3 — FetchSource / AddSource

**Goal:** node outputs become addressable knowledge. `dear-agent search
"topic"` returns sources cited by previous research runs, joined to
their work-items.
**Phase status:** `done`
**Estimated:** 2 weeks
**Depends on:** Phase 0 (parallelizable with Phase 1 + Phase 2)

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 3.1 | `pkg/source` adapter interface | `pkg/source/adapter.go` | Interface matches ADR-010 §D9 contract; documented godoc | — | S | `done` |
| 3.2 | SQLite + FTS5 adapter | `pkg/source/sqlite/adapter.go`, `adapter_test.go` | FTS round-trip; 10K-row Fetch P95 < 50 ms | 3.1 | M | `done` |
| 3.3 | MCP tools `FetchSource` / `AddSource` | `cmd/dear-agent-mcp/source.go` | Tools call through adapter; reject backend mismatch with clear error | 3.2 | S | `done` |
| 3.4 | Wire `outputs.durability=engram_indexed` to `AddSource` | `pkg/workflow/outputs.go` (extends 1.6) | Run produces a row in `sources` table per node-output declared `engram_indexed` | 3.3, 1.6 | S | `done` |
| 3.5 | `dear-agent search` CLI | `cmd/dear-agent-search/main.go` | Returns results joined to `runs.run_id`; `--since` flag filters | 3.4 | S | `done` |

**Phase 3 ship criterion:** `dear-agent search` returns results from the
last 30 days of research outputs, joined to their work-item ids; FTS
round-trip P95 < 50 ms on 10K rows.

---

## Phase 4 — Migration + `workflow dev`

**Goal:** real workloads run on the engine; the inner-loop iteration
experience matches the synthesis's "10-minute walkthrough" target.
**Phase status:** `done — all five tickets landed`
**Estimated:** 4 weeks
**Depends on:** Phases 1, 2, and 3

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 4.1 | Codemod tool: v0.1 → v0.2 workflow upgrade | `pkg/workflow/codemod/codemod.go`, `cmd/workflow-codemod/main.go` | Adds `schema_version: "1"`, promotes known `model:` → `role:`, optional default budget; dry-run by default, `--write` overwrites; tested against synthetic v0.1 fixtures because no shipped v0.1 research workflow exists yet | 1.*, 3.* | S | `done` |
| 4.2 | Codemod tool: Wayfinder session → workflow synthesis | `pkg/workflow/codemod/wayfinder.go`, `cmd/workflow-codemod/main.go` (`from-wayfinder` subcommand) | Reads a Wayfinder session YAML; emits a workflow YAML where each roadmap phase becomes a bash node with linear depends; passes lint and round-trips through `workflow.LoadBytes` | 2.* | M | `done` |
| 4.3 | Deprecate JSON `FileState` path; provide migration tool | `cmd/workflow-migrate/main.go` | Old snapshot → SQLite db; round-trip preserves all fields | 0.* | S | `done` |
| 4.4 | `workflow dev` interactive mode | `cmd/workflow-dev/main.go`, `pkg/workflow/dev/` | Hot-reload (fsnotify); mock-by-default fixture executor; verbs `r [--live] / retry <node> / diff <node> / approve <id> / reload / fixtures / nodes / history / help / exit`; sub-second iteration when running with mocks | 1.*, 2.* | L | `done` |
| 4.5 | Documentation: `docs/workflow-engine.md` | new file | Includes the 10-minute walkthrough; covers role registry, HITL, outputs, search | all | S | `done` |

**Phase 4 ship criterion:** new user goes `brew install` → useful workflow
in ten minutes. Recorded fixtures make iteration sub-second. All existing
workflows have been migrated and lint-clean.

---

## Phase 5 — Adapters + visual inspector + `kind: spawn`

**Goal:** the engine is extensible. Plugin packaging keeps the core small.
**Phase status:** `done — six tickets landed architecturally; OpenViking ships as a stub`
**Estimated:** open-ended; ship items as demand surfaces

> **2026-05-03 follow-up:** broken-windows pass. Every Phase 5 ticket
> ships now even where no external driver exists, because leaving the
> shape unbuilt invites contradictory assumptions later. The OpenViking
> stub returns `ErrNotImplemented` from every method but satisfies the
> interface so callers compile against a stable contract; the
> registry treats its presence as a feature flag, not a working
> backend.

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 5.1 | Obsidian adapter (single-user dual-write) | `pkg/source/obsidian/` | Write to vault as markdown + YAML frontmatter; Fetch via vault-walk + substring; passes the `pkg/source/contract` suite | 3.1 | M | `done` |
| 5.2 | llm-wiki adapter (markdown + git) | `pkg/source/llmwiki/` | Write to wiki dir; AutoCommit when inside a git repo (graceful when not); passes the contract suite; verified end-to-end with a real git repo | 3.1 | M | `done` |
| 5.3 | OpenViking adapter (graph DB; future / enterprise) | `pkg/source/openviking/` | Stub: Adapter satisfies `source.Adapter` and returns `ErrNotImplemented` from every method; `Config` shape defined; registry registers it as a feature-flag entry | 3.1 | L | `done — stub` |
| 5.4 | `kind: spawn` for emergent DAG growth | `pkg/workflow/types.go`, `pkg/workflow/spawn.go`, `runner.go` | Spawn body cmd emits YAML node list; runner validates, topo-sorts, and executes inline; cycle detection over spawned subgraph; max_children + allowed_kinds guards; spawned ids namespaced as `<parent>/<child>` so audit rows distinguish them | 1.* | M | `done` |
| 5.5 | Visual run inspector (web UI reading SQLite) | `cmd/workflow-inspector/` | Read-only HTTP server; lists runs with state filter; per-run drill-down with nodes + audit timeline; no authoring; binds loopback by default | 0.* | L | `done` |
| 5.6 | Plugin packaging (registry indirection) | `pkg/source/registry/` | `Register(name, factory)` indirection; built-in adapters auto-register from `builtins.go`; plugin authors register from their own package's init; `Open(name, config)` returns the right adapter or "unknown backend" | 5.1+ | M | `done` |

---

## Cross-phase / not-yet-categorized

Tickets that surfaced during research but don't fit cleanly into a
phase. Triage as they come up.

| # | Title | Notes |
|---|---|---|
| X.1 | Schema evolution policy (additive vs. ALTER) | ADR open question §5; needs a decision before Phase 2 lands |
| X.2 | Cost-per-mtok refresh in `roles.yaml` | ADR open question §3; punted to post-MVS |
| X.3 | Per-tenant isolation | ADR open question §2; punted to post-MVS |
| X.4 | GitHub-PR HITL backend | ADR open question §4; not in v1 |
| DEAR-X.5 | ~~Flaky `TestSQLiteStateConcurrentSaves` (SQLITE_BUSY on schema apply)~~ DONE | Fixed by `pingWithBusyRetry` + `execWithBusyRetry` retry loops in `state_sqlite.go` (see `openSQLiteDB` and `retryOnSQLiteBusy`). 100-iteration soak test now passes (`go test -count=100 -run TestSQLiteStateConcurrentSaves ./pkg/workflow/`). |
| DEAR-X.6 | ~~Phase 2 wiring for Phase 1 schema fields~~ DONE | Wired by Phase 2.* tickets (#40). Runner now exposes `Permissions` and HITL backend hooks (`pkg/workflow/runner.go`) and the audit pipeline emits transition rows for permission denials and HITL approve/reject/timeout. |

---

## Substrate test suite (mandatory contract tests)

These tests must pass for any `State`, `Adapter`, or `Runner` implementation.
They live in `pkg/workflow/contract/` and are added in Phase 0 (state
contracts) + Phase 1 (runner contracts) + Phase 3 (adapter contracts).

```
// State contract:
TestState_Save_AtomicOnCrash
TestState_Load_NonExistent_ReturnsNil
TestState_Save_Concurrent_NoCorruption
TestState_QueryRunsByState_PerformanceTarget

// Runner contract — state machine:
TestRunner_NodeStates_Transition_PendingRunningSucceeded
TestRunner_NodeStates_Transition_RunningFailedRetried
TestRunner_NodeStates_Transition_RunningAwaitingHITL_Approve
TestRunner_NodeStates_Transition_RunningAwaitingHITL_Reject
TestRunner_NodeStates_Transition_RunningAwaitingHITL_Timeout
TestRunner_NodeStates_Skipped_OnUpstreamFail
TestRunner_NodeStates_Skipped_OnFalseWhen
TestRunner_AllTransitions_EmitAuditRow

// Runner contract — gates:
TestRunner_ExitGate_BashFailure_BlocksSuccess
TestRunner_ExitGate_SchemaInvalid_BlocksSuccess
TestRunner_ExitGate_OrderedShortCircuit
TestRunner_ExitGate_PassesAllSuccess

// Runner contract — budget:
TestRunner_Budget_TokenCeiling_FailsNode
TestRunner_Budget_DollarCeiling_FailsNode
TestRunner_Budget_WallclockCeiling_FailsNode
TestRunner_Budget_OnOverrun_Escalate
TestRunner_Budget_OnOverrun_Truncate
TestRunner_Budget_OnOverrun_Fail

// Runner contract — permissions:
TestRunner_Permissions_RejectedTool_FailsNode
TestRunner_Permissions_OutOfPathWrite_FailsNode
TestRunner_Permissions_NetworkAllowlist_RejectsNonAllowed

// Runner contract — retry:
TestRunner_Retry_TransientError_Retries
TestRunner_Retry_RateLimit_BackoffRespected
TestRunner_Retry_NonRetryableError_FailsImmediate
TestRunner_Retry_RetryOnList_FiltersErrors
TestRunner_Retry_AttemptRowsPersisted

// Adapter contract:
TestAdapter_AddFetch_RoundTrip
TestAdapter_FetchByCue_Filters
TestAdapter_FetchByWorkItem_Filters
TestAdapter_HealthCheck
```

A new state, adapter, or runner shipping without these tests is not
substrate-quality and should be rejected at PR review.

---

## Coverage targets

| Package | Target |
|---|---|
| `pkg/workflow` | 85% |
| `pkg/workflow/roles` | 95% |
| `pkg/source` | 90% |
| `pkg/source/sqlite` | 90% |

---

## References

- [ROADMAP.md](../../ROADMAP.md)
- [ADR-010](../adrs/ADR-010-workflow-engine-architecture.md)
- `~/src/engram-research/WORKFLOW-ENGINE-RESEARCH-ENGINEERING.md` §9 (origin/main, 2026-05-02) — original ticket source
- `~/src/engram-research/WORKFLOW-ENGINE-SYNTHESIS.md` §7 (origin/main, 2026-05-02) — phase rationale
