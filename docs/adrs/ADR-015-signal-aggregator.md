# ADR-015: Signal Aggregator for the Recommendation Engine

**Status**: Proposed
**Date**: 2026-05-03
**Context**: dear-agent currently answers "what is the workflow doing?"
extremely well (the runs.db substrate from ADR-010 records every node
execution). It does *not* answer "what should we work on next?" — there
is nowhere a session can ask "is lint trending up? are deps stale? where
is coverage thin?". The context-engine track on the roadmap calls for a
recommendation MCP server that fuses these inputs into a ranked list of
work suggestions. That server needs an aggregated, time-series view of
project health to pull from. This ADR introduces the data layer that
will feed it.

Builds on:
- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
  — establishes SQLite as the substrate-quality storage layer and the
  pattern the aggregator follows.
- [ADR-014: Plugin System](ADR-014-plugin-system.md) — the aggregator's
  collector interface is shaped so a future plugin capability
  (`SignalCollector`) can register third-party collectors without
  touching the core.

---

## Context

A recommendation engine that says "fix lint, then refresh deps, then add
coverage to `pkg/foo`" needs three things its consumers do not currently
have:

1. **Heterogeneous inputs in one place.** Git activity, lint counts, test
   coverage, dependency freshness, and security alerts live in
   different tools with different output formats (`git log`, `golangci-lint
   run --output-format=json`, `go test -cover`, `go list -u -m`,
   `govulncheck`). Reading them on demand each time a recommendation is
   computed is slow, and it loses the time-series view ("is lint going
   up or down this week?").
2. **A persistent time-series store.** Recommendations care about
   *trends*. A package whose lint count is stable at 12 is not the same
   as a package whose count was 2 last week and is 12 today. Without a
   persistent store every consumer reinvents the windowing logic.
3. **A weighted scorer.** Different signals matter differently for
   different repos: in one repo a single security alert dominates, in
   another a 30% drop in coverage is the priority. The recommendation
   engine needs a single number per signal kind it can rank on, with
   weights that an operator can tune.

The closest existing primitive is `pkg/workflow.SQLiteState` (ADR-010 §5),
which already proves out the substrate pattern: one SQLite file, a
typed Go API, plain text and JSON CLIs, busy-retry semantics. The
aggregator reuses that pattern verbatim — it is the same shape with a
different schema.

Three concrete shortcomings drive this ADR:

1. **No place to put trend data.** runs.db is the work-item store; it
   should not absorb cross-cutting health metrics. A second SQLite
   file is the right size of separation.
2. **Collectors today are CLIs run by humans.** Each tool exists as a
   one-shot CLI invocation. Nothing reads them programmatically, runs
   them on a schedule, or persists their output for later comparison.
3. **No shared scoring vocabulary.** The recommendation engine, the
   audit subsystem, and a future "what to deprecate?" tool will all
   want the same weighted-priority story. Building it once, here, is
   strictly cheaper than building it three times in three consumers.

---

## Decision

Introduce a new package `pkg/aggregator/` that provides a `Collector`
interface, a SQLite-backed `Store`, an `Aggregator` runner, and a
`Scorer` for weighted priority. Five first-party collectors ship in
Phase 1: `GitActivity`, `LintTrend`, `TestCoverage`, `DepFreshness`,
`SecurityAlerts`. A new CLI binary `cmd/dear-agent-signals` exposes
`collect` and `report` subcommands.

### D1. Package name: `pkg/aggregator`, not `pkg/signals`

`pkg/signals/` already exists in this repo with a different meaning —
it implements complexity-signal detection for Hybrid Progressive Rigor
(keyword/effort/file-type fusion that produces a recommended rigor
level). That package is consumed by the audit and rigor pipeline; it is
*not* the project-health signal store this ADR is about.

Rather than rename a package that already has consumers, we put the
new layer at `pkg/aggregator/`. The exported type is `Signal`; the
domain noun ("project signal", as opposed to "rigor signal") is carried
by the package path, not the type name. A future ADR may consolidate
the two if a clear superset emerges; until then, the names stay
distinct so the imports tell readers which kind of signal they are
looking at.

### D2. The `Signal` type and `Kind` enum

```go
type Signal struct {
    ID          string    // UUID, generated on insert
    Kind        Kind      // see below
    Subject     string    // free-form: package path, module, file, dep name
    Value       float64   // numeric observation (count, percentage, ratio)
    Metadata    string    // JSON-encoded extras for collectors that need more
    CollectedAt time.Time // wall-clock at collection
}

type Kind string

const (
    KindGitActivity    Kind = "git_activity"
    KindLintTrend      Kind = "lint_trend"
    KindTestCoverage   Kind = "test_coverage"
    KindDepFreshness   Kind = "dep_freshness"
    KindSecurityAlerts Kind = "security_alerts"
)
```

`Subject` is intentionally a free-form string. A `git_activity` row's
subject is the repo path; a `test_coverage` row's subject is a Go
package import path; a `dep_freshness` row's subject is a module path.
The schema does not try to model the relationships across signal kinds
because the recommendation engine does not need them: ranking is
per-kind first, weighted-summed second.

`Metadata` is JSON-encoded to keep the schema flat. Collectors that
want to record (for example) the specific lint rule IDs that fired
serialize them into Metadata; the report CLI pretty-prints, the
recommendation engine ignores it. Storing JSON in a TEXT column matches
how `pkg/workflow` stores `inputs_json` and `audit_events.payload_json`.

### D3. The `Collector` interface

```go
type Collector interface {
    Name() string                          // stable identifier, e.g. "git.activity"
    Kind() Kind                            // which Kind this collector emits
    Collect(ctx context.Context) ([]Signal, error)
}
```

A collector is independent of the store: it produces signals, the
aggregator persists them. This separation matters for two reasons:

1. **Testability.** Each collector is testable in isolation by calling
   `Collect` and asserting on the returned signals — no SQLite
   required in the unit tests.
2. **Compositionality.** The plugin system (ADR-014) can introduce a
   `SignalCollector` capability later that registers third-party
   collectors. The `Aggregator` then just iterates them; the schema
   does not change.

`Name()` is the dot-separated identifier convention from
[ADR-014 §D2](ADR-014-plugin-system.md). First-party collectors use
`dear-agent.<area>`: `dear-agent.git`, `dear-agent.lint`, etc.

### D4. The `Store` interface and SQLite schema

```go
type Store interface {
    Insert(ctx context.Context, sigs []Signal) error
    Recent(ctx context.Context, kind Kind, limit int) ([]Signal, error)
    Range(ctx context.Context, kind Kind, since time.Time) ([]Signal, error)
    Kinds(ctx context.Context) ([]Kind, error)
    Close() error
}
```

The schema is one table:

```sql
CREATE TABLE IF NOT EXISTS signals (
    signal_id     TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    subject       TEXT NOT NULL,
    value         REAL NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    collected_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_signals_kind_at
    ON signals (kind, collected_at);
CREATE INDEX IF NOT EXISTS idx_signals_subject
    ON signals (subject);
```

One table, two indexes — that is enough to serve every Phase 1 query
the recommendation engine and the report CLI need. The index on
`(kind, collected_at)` covers `Recent` and `Range`; the index on
`subject` covers per-package drill-down.

The driver is `modernc.org/sqlite` (matches `pkg/workflow`); the DSN
pragmas are the same `busy_timeout(5000) + journal_mode(WAL) +
foreign_keys(on)` triple. There is one schema file, applied
idempotently on every open. No migrations: the column set is small
enough that schema evolution will land via a numbered migration ADR if
and when it becomes necessary.

### D5. The `Aggregator`

```go
type Aggregator struct {
    Store      Store
    Collectors []Collector
    Now        func() time.Time   // overridable in tests
}

func (a *Aggregator) Run(ctx context.Context) (Report, error)

type Report struct {
    StartedAt   time.Time
    FinishedAt  time.Time
    Collected   map[string]int   // collector name → signals emitted
    Errors      map[string]error // collector name → first error (nil if ok)
}
```

`Run` invokes every registered collector in registration order, persists
the resulting signals, and returns a per-collector summary. A failing
collector does **not** fail the run: its error is recorded in the
report and other collectors continue. This matches the "audit emission
is unconditional" guarantee from
[ADR-010 §D3](ADR-010-workflow-engine-architecture.md): partial data
beats no data, and the report makes failures visible.

Concurrency in v1 is sequential. Five Phase 1 collectors are cheap
enough (typical run is well under a second on a small repo) that the
serialization is not a bottleneck. A future change can fan them out;
the interface does not preclude it.

### D6. The `Scorer`

```go
type Scorer struct {
    Weights map[Kind]float64    // 0..1, sums need not equal 1
}

type Score struct {
    Kind     Kind
    Raw      float64    // collector-defined "pressure" (e.g. lint count)
    Norm     float64    // 0..1, normalized within the recent window
    Weight   float64    // from Scorer.Weights
    Weighted float64    // Norm * Weight
}

func (s Scorer) Score(signals []Signal) []Score
func (s Scorer) Total(scores []Score) float64
```

The scorer takes the most recent signal per kind, normalizes its value
to 0..1 against a reasonable per-kind ceiling (lint: clamp at 200,
coverage: invert against 100, deps: clamp at 50, security: clamp at
10, git_activity: clamp at 100 commits/week), multiplies by the
configured weight, and sums. The clamping ceilings are
`scorer.go`-internal constants in v1; the recommendation engine ADR
will revisit whether they need to be data-driven once we have a few
weeks of real signals to look at.

Defaults if `Weights` is nil:

```go
DefaultWeights = map[Kind]float64{
    KindSecurityAlerts: 1.0,   // dominates by design
    KindLintTrend:      0.4,
    KindTestCoverage:   0.5,
    KindDepFreshness:   0.3,
    KindGitActivity:    0.2,
}
```

These defaults are operator hints, not policy. They live alongside the
type so a CLI flag or config file can override them without touching
the package.

### D7. Five Phase 1 collectors

Each collector accepts an `Exec` indirection
(`func(ctx, name string, args ...string) ([]byte, error)`) so unit
tests can fake out the external command. Production wires
`exec.CommandContext` and pipes stdout into the supplied buffer. This
is the same pattern `pkg/audit` uses for its check runners.

| Collector | External tool | Subject form | Value semantics |
|---|---|---|---|
| `dear-agent.git` | `git log --since=<window>` | repo root path | commits in window |
| `dear-agent.lint` | `golangci-lint run --output.json.path=stdout` | per-file path | finding count for that file |
| `dear-agent.coverage` | `go test -cover ./...` (or coverage file) | Go import path | coverage percent |
| `dear-agent.deps` | `go list -u -m -json all` | module path | 1 if outdated, 0 otherwise |
| `dear-agent.security` | `govulncheck -json ./...` | vuln ID (e.g. `GO-2024-1234`) | 1 (presence) |

**Failure modes for missing tools.** If a collector's external command
is not on `$PATH`, `Collect` returns a typed error
(`ErrToolMissing{Tool: "golangci-lint"}`) rather than panicking. The
aggregator records it in the per-run report and the operator gets a
clear message from `dear-agent-signals collect`. This matters for
lightweight dev environments that may not have every tool installed.

**Coverage as input vs. invocation.** `dear-agent.coverage` accepts
either an already-computed coverage profile path
(`-coverprofile=cover.out` from a CI run) or invokes `go test
-cover ./...` itself. CI runs typically already have coverage data on
disk; reusing it is faster and avoids running tests twice. The CLI
takes `--coverage-file` to opt into the input form.

### D8. CLI surface

One new binary, `cmd/dear-agent-signals`, with two subcommands:

```
dear-agent-signals collect [--db PATH] [--repo PATH] [--coverage-file PATH]
dear-agent-signals report  [--db PATH] [--kind KIND] [--since DURATION]
                           [--json] [--limit N]
```

Convention follows the existing `workflow-list` / `workflow-status`
binaries: `flag` package, `--db` defaults to `./signals.db`, `--json`
emits machine-readable output, plain text uses `tabwriter`.

`collect` runs the aggregator once and prints a per-collector summary
(rows-emitted and any errors). It is the primitive that a cron job or
a `dear-audit` post-run hook calls.

`report` reads the store and prints either a per-kind summary (default)
or the recent signals for a single kind (`--kind`). The recommendation
MCP server will use the package directly rather than shelling to this
CLI, but the CLI exists for operators and for manual debugging.

### D9. What this ADR does not do

Out of scope for v1:

- **The recommendation engine itself.** This ADR delivers the data
  layer; ranking signals into a "what to work on next" list is a
  separate ADR (planned: ADR-016).
- **Streaming / push collection.** Phase 1 is poll-only:
  `Aggregator.Run` is a one-shot. A future change can introduce a
  long-running collection daemon (similar to `pkg/vroom`).
- **Per-author signals.** Git-activity rolls up per repo, not per
  contributor. Author attribution is its own privacy/permissions
  conversation we are not having yet.
- **Cross-repo aggregation.** One signals.db, one repo. Operators that
  want a fleet view will collect into per-repo files and merge with a
  separate tool.
- **Plugin-registered collectors.** ADR-014 reserves a `SignalCollector`
  capability name; this ADR ships the data structures it will
  eventually plug into, but does not yet add the plugin glue.

### D10. Naming and config conventions

- Package: `pkg/aggregator/` (singular). The exported type is `Signal`,
  matching the domain noun.
- Default DB filename: `signals.db`. Per-repo, lives at
  `./.dear-agent/signals.db` if the directory exists, falls back to
  `./signals.db` otherwise.
- Collector name namespace: `dear-agent.<area>` for first-party,
  reverse-DNS for third-party (matches ADR-014 §D8).
- CLI binary: `cmd/dear-agent-signals/` (single binary, internal
  subcommand routing — distinct from the existing
  `workflow-*` per-binary pattern because the surface is small enough
  that two binaries would be wasteful).

---

## Architecture diagram

```
                  Caller (cmd/dear-agent-signals or MCP server)
                              │
                              │ 1. construct collectors,
                              │    Aggregator{Store, Collectors}
                              ▼
                  ┌──────────────────────────┐
                  │      Aggregator.Run      │
                  └──────────┬───────────────┘
                             │
            ┌────────────────┼─────────────────┐
            ▼                ▼                 ▼
      Collector       Collector            Collector
      .git.activity   .lint.trend          .security.alerts
            │                │                 │
            └─── Signal[]  ──┴─────────────────┘
                             │
                             ▼
                  ┌──────────────────────────┐
                  │   Store.Insert(signals)  │
                  │   (SQLite signals.db)    │
                  └──────────┬───────────────┘
                             │
                             │ later, on demand
                             ▼
                  ┌──────────────────────────┐
                  │  Scorer.Score(signals)   │
                  │  → per-kind weighted     │
                  │    priority for the      │
                  │    recommendation engine │
                  └──────────────────────────┘
```

---

## Consequences

**Positive.**

- A recommendation engine has a stable, queryable substrate. The MCP
  server and any future "what to work on next" tool talk to the same
  store with the same types.
- Collectors are independently testable and independently failable.
  A broken `golangci-lint` invocation does not poison the rest of the
  pipeline.
- The schema is small enough that swapping out the store
  (e.g. for a remote DB later) is a few hundred lines of work, not a
  rewrite.
- The plugin-system reservation
  ([ADR-014 §D7](ADR-014-plugin-system.md)) means a future
  `SignalCollector` capability slots in without breaking existing
  collectors or the schema.

**Negative.**

- A second SQLite file (in addition to `runs.db`) means operators have
  two stores to back up and reason about. We accept this — the
  semantics are different enough that consolidating would muddle both.
- The `Subject` field is free-form. Cross-kind queries (e.g. "show me
  every signal for `pkg/foo`") rely on collectors using consistent
  subject conventions. We document the conventions in `aggregator/doc.go`;
  enforcement is a future concern.
- Scorer weights and clamping ceilings are hardcoded constants in v1.
  Operators that want to tune them must wait for the
  recommendation-engine ADR or pass `Scorer{Weights: ...}` directly.

**Migration path.** None — this is a new surface. No existing data,
no existing consumers. The recommendation engine ADR (ADR-016, planned)
will document how it queries the store; until then, the package
contract is the source of truth.

---

## Validation

The package is correct when:

1. `Aggregator.Run` with N working collectors produces N batches of
   signals, all persisted with the same `CollectedAt` window.
2. `Aggregator.Run` with a failing collector still persists the
   working collectors' signals and reports the failure.
3. `Store.Range(kind, since)` returns exactly the signals collected
   in `[since, now]`.
4. `Scorer.Score` is monotone in raw value within a kind (more lint
   findings → higher Norm → higher Weighted, holding weights constant).
5. The CLI's `collect` and `report` subcommands have golden tests
   against a deterministic in-memory clock.

Phase 1 is done when those five properties have tests, the binary
builds, and `go vet ./... && golangci-lint run ./...` passes clean.

---

## References

- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md) — SQLite substrate pattern.
- [ADR-014: Plugin System](ADR-014-plugin-system.md) — `SignalCollector` capability reservation.
- `pkg/workflow/state_sqlite.go` — driver, pragmas, busy-retry pattern this ADR mirrors.
