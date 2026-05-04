# ADR-016: Recommendation MCP Server

**Status**: Proposed
**Date**: 2026-05-03
**Context**: [ADR-015](ADR-015-signal-aggregator.md) shipped the data layer
that records project-health signals (git activity, lint, coverage, deps,
security) into a SQLite store with a weighted scorer. ADR-015 deliberately
left the consumer surface out of scope: "ranking signals into a 'what to
work on next' list is a separate ADR (planned: ADR-016)." This ADR is that
consumer. It introduces an MCP server that exposes the aggregator over
JSON-RPC so any MCP client (Claude Code, Cursor, custom agents) can ask
"what should we work on next?" without knowing the SQLite schema.

Builds on:

- [ADR-015: Signal Aggregator](ADR-015-signal-aggregator.md) — the data
  layer this server queries. The package contract (`Store`, `Signal`,
  `Scorer`) is the source of truth; this ADR adds an RPC skin.
- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
  — `cmd/dear-agent-mcp` proves the JSON-RPC-over-stdio pattern this
  server reuses verbatim.

---

## Context

The aggregator is queryable by Go code today, and by the
`dear-agent-signals report` CLI for human operators. Neither surface is
useful to an MCP client: an LLM session that wants "what should we work
on next?" cannot import a Go package and cannot reliably parse the
report CLI's tabwriter output across versions. The recommendation surface
must be MCP-native.

Three concrete needs drive this ADR:

1. **An MCP-native query path.** A Claude Code session opens an MCP
   server over stdio and calls `tools/call`. There is no equivalent in
   the current dear-agent surface for "ask the aggregator" — only
   `dear-agent-mcp` (workflow engine + sources) and `gopls`
   (language server) are wired into `.mcp.json`.
2. **A ranked recommendation, not raw signals.** A consumer that wants
   "the top three things to fix" does not want to compute weighted
   scores client-side. The aggregator's `Scorer` already does this; the
   server should expose the result, not push the math onto every client.
3. **Trend visibility.** A signal whose value is 12 today is meaningless
   without "what was it last week?". The aggregator's `Range` query
   delivers the raw points; the server should bucket them so clients
   render trends without re-implementing time math.

A separate binary (rather than adding tools to `dear-agent-mcp`) keeps
concerns clean: `dear-agent-mcp` owns *write* paths into runs.db and
sources.db; this server is read-only against signals.db. They share the
JSON-RPC dispatch shape but neither depends on the other, and
`signals.db` may live on a different host than `runs.db` (CI fleet vs
developer laptop) once the operator wants that.

---

## Decision

Introduce `cmd/recommendation-mcp/` — a Go binary that opens a
read-only `aggregator.SQLiteStore` and serves three MCP tools over
stdio. The dispatch shape and error-code conventions are copied from
`cmd/dear-agent-mcp` so any future MCP machinery (auth, tracing, batch
calls) lands once and applies to both servers.

### D1. Three tools, all read-only

| Tool                  | Purpose                                                    |
|-----------------------|------------------------------------------------------------|
| `get_signals`         | Filtered query against the store (kind, subject, since)    |
| `get_recommendations` | Ranked priority list (top-N weighted scores)               |
| `get_signal_trends`   | Time-bucketed aggregation for a single kind                |

This server never writes. Collection is the aggregator's job
(`dear-agent-signals collect`); mutation tools belong on a separate
surface if and when one is needed. Read-only is enforced at the boundary
by *not exposing* a `write` method, not by sniffing SQLite open flags —
but the open path uses `?mode=ro` where supported as defense in depth.

### D2. `get_signals` — primitive query

```jsonc
// Input
{
  "kind": "lint_trend",        // optional; omit to query all kinds
  "subject": "pkg/foo/bar.go", // optional substring filter
  "since": "2026-04-01T00:00:00Z", // optional RFC3339 lower bound
  "limit": 100                 // optional, default 100, max 1000
}
// Output
{
  "kind": "lint_trend",
  "signals": [
    {"id":"...","kind":"lint_trend","subject":"...","value":12,
     "metadata":"{...}","collectedAt":"2026-05-03T12:00:00Z"}
  ]
}
```

When `kind` is omitted, the server fans the query out across every kind
known to the store (`Store.Kinds`) and concatenates results. The
`limit` cap (max 1000) is the ADR-010 §D8 convention: the JSON-RPC
transport is one envelope per response, and >1000 rows pushes against
both stdio buffer sizes and client-side render budgets. Callers that
genuinely need bulk export should query the SQLite file directly.

`subject` filtering is substring-based on the SQLite side (`subject
LIKE '%' || ? || '%'`) because that matches how the aggregator's
free-form Subject convention is consumed: an operator types
`pkg/foo/` and expects every file under that package. Indexed prefix
matching is a future optimization once we have real-world usage data.

### D3. `get_recommendations` — ranked priorities

```jsonc
// Input
{
  "top_n": 10,                 // optional, default 10, max 50
  "window": "168h",            // optional Go duration, default 168h (7 days)
  "weights": {                 // optional override map[Kind]float64
    "security_alerts": 1.0,
    "test_coverage": 0.6
  }
}
// Output
{
  "generated_at": "2026-05-03T12:00:00Z",
  "window": "168h",
  "total_score": 3.42,
  "recommendations": [
    {
      "kind": "security_alerts",
      "subject": "GO-2024-1234",
      "raw": 1, "norm": 0.1, "weight": 1.0, "weighted": 0.1,
      "summary": "security_alerts on GO-2024-1234 (raw=1)"
    }
  ]
}
```

The ranking algorithm:

1. Pull every kind known to the store (`Store.Kinds`).
2. For each kind, fetch signals collected within `window`
   (`Store.Range(kind, now-window)`).
3. Reduce to *most recent per (kind, subject)* — a file with two lint
   observations in the window contributes one row, not two.
4. Run `aggregator.Scorer{Weights: ...}.Score(...)` over the reduced
   set. The scorer already sorts by `Weighted` descending.
5. Truncate to `top_n` and emit.

The "most recent per (kind, subject)" reduction is the right default
for a recommendation: an operator wants to know the *current* state of
each subject, not a sum of historical observations. (Trend questions
go to `get_signal_trends` instead.)

`weights` overrides go into `Scorer.Weights`; missing keys still fall
back to `aggregator.DefaultWeights` per ADR-015 §D6. The client never
needs to enumerate every kind to override one.

`summary` is a one-line human-readable rationale generated server-side
so MCP clients can render a recommendation list without owning the
formatting. The format is intentionally machine-stable
(`<kind> on <subject> (raw=<value>)`) — operators that want richer
prose layer it on top.

### D4. `get_signal_trends` — time-bucketed aggregation

```jsonc
// Input
{
  "kind": "lint_trend",        // required
  "subject": "pkg/foo",        // optional substring filter
  "window": "720h",            // optional Go duration, default 720h (30 days)
  "bucket": "24h"              // optional Go duration, default 24h, min 1h
}
// Output
{
  "kind": "lint_trend",
  "window": "720h",
  "bucket": "24h",
  "buckets": [
    {"start":"2026-04-03T00:00:00Z","end":"2026-04-04T00:00:00Z",
     "count":3,"mean":11.33,"min":10,"max":13}
  ]
}
```

Bucketing happens in Go after `Store.Range(kind, since)` returns. We
do not push bucketing into SQL because:

1. The math is cheap (every bucket aggregation is one pass over the
   range), and one pass keeps the substrate query trivial.
2. Bucket boundaries that align to the *caller's* clock (typically
   UTC midnight) matter more than bucket boundaries that align to
   `collected_at`'s clock. Doing the bucket-edge math in Go lets the
   server use whatever zero-point the client expects without growing
   SQL dialect surface.

`bucket` minimum of 1h prevents a misconfigured client from asking for
a million 1-second buckets; `window/bucket > 1000` returns
`-32602 invalid arguments`. (Same defense-in-depth rationale as the
`get_signals` row cap.)

Empty buckets are emitted: a window of 30 days with daily buckets
returns 30 entries even if some are zero-count. The recommendation
engine charts these directly; trimming would push the "did the signal
disappear or did we lose collection?" disambiguation onto every client.

### D5. Server lifecycle and the `--db` flag

```
recommendation-mcp [--db PATH] [--http ADDR]
```

`--db` defaults to `./signals.db` (matches the aggregator's default
from ADR-015 §D10). The server opens it with
`OpenSQLiteStore` and applies the schema idempotently on startup, so
a fresh deployment that has never run `dear-agent-signals collect`
still serves `tools/list` correctly (it just returns empty results
from every query). This matters for the MCP client UX: "the server
is up and reachable" should not fail because no signals have been
collected yet.

`--http ADDR` is the same test/debug shim `dear-agent-mcp` ships:
serve one JSON-RPC envelope per HTTP request at `/jsonrpc`. Production
deployments use stdio.

The server holds *one* `*aggregator.SQLiteStore` for its lifetime.
SQLite WAL mode (set by `aggregator.OpenSQLiteStore`) handles
concurrent reads from this server alongside concurrent writes from
`dear-agent-signals collect` running on a cron.

### D6. Error-code mapping

Inherits the JSON-RPC code conventions from `dear-agent-mcp`:

| Code   | Meaning                                                |
|--------|--------------------------------------------------------|
| -32700 | parse error (request not valid JSON)                   |
| -32600 | invalid request (envelope shape wrong)                 |
| -32601 | method not found / unknown tool                        |
| -32602 | invalid arguments (bad kind, bad RFC3339, oversize N)  |
| -32000 | generic server error (DB open failed, query failed)    |
| -32001 | not found (kind not in store)                          |

Unknown `kind` strings return `-32602` (rejected at the boundary by
`Kind.Validate`); a known but unrecorded kind returns an empty result
with `-32001` so a client can distinguish "you asked for the wrong
thing" from "we have nothing yet."

### D7. `.mcp.json` entry

```jsonc
{
  "mcpServers": {
    "gopls":  { "type": "stdio", "command": "gopls",            "args": ["mcp"] },
    "dear-agent-recommendations": {
      "type": "stdio",
      "command": "recommendation-mcp",
      "args": ["--db", ".dear-agent/signals.db"]
    }
  }
}
```

The server name `dear-agent-recommendations` (not `recommendation-mcp`)
matches the ADR-015 §D10 convention: tool surfaces use the
`dear-agent-<area>` namespace; the binary name is an implementation
detail. Clients see the namespaced name; the binary on disk is what
`PATH` resolves.

### D8. What this ADR does not do

Out of scope for v1:

- **A long-running collection daemon.** This server reads what
  `dear-agent-signals collect` wrote. Operators run that on a cron or
  a post-commit hook; the recommendation server never collects.
- **Cross-repo aggregation.** One signals.db, one recommendation
  surface. A multi-repo recommendation engine is a future ADR.
- **Recommendation history.** The server computes recommendations
  on demand from the latest snapshot. We do not persist
  "yesterday's top-3" — that is a query against `get_signal_trends`
  for each kind plus client-side reconstitution.
- **Authentication.** Stdio is local-process; HTTP is dev-only. Once
  the server gets a remote production deployment, an auth ADR lands
  alongside it.

---

## Architecture diagram

```
   MCP client (Claude Code, Cursor)
              │
              │ JSON-RPC 2.0 over stdio
              ▼
   ┌──────────────────────────────┐
   │   recommendation-mcp         │
   │                              │
   │   tools/list                 │
   │   tools/call                 │
   │     ├─ get_signals           │
   │     ├─ get_recommendations   │
   │     └─ get_signal_trends     │
   └──────────────┬───────────────┘
                  │ aggregator.Store
                  ▼
   ┌──────────────────────────────┐
   │   pkg/aggregator             │
   │   SQLiteStore (signals.db)   │  ← written by dear-agent-signals
   │   Scorer (DefaultWeights)    │
   └──────────────────────────────┘
```

---

## Consequences

**Positive.**

- A Claude Code session asks "what should we work on next?" via one
  tool call; no shell, no parsing.
- Read-only by construction. A misbehaving MCP client cannot corrupt
  signals.db through this server.
- The split between `dear-agent-mcp` (writes runs.db) and
  `recommendation-mcp` (reads signals.db) keeps each server's blast
  radius small and lets operators deploy them independently.
- The same JSON-RPC scaffolding pattern as `dear-agent-mcp` means a
  future MCP-wide change (capability negotiation, batching) lands in
  both servers via the same edit.

**Negative.**

- A second MCP binary to install, configure, and version. We accept
  this — folding the tools into `dear-agent-mcp` would couple the
  workflow engine's release cadence to the recommendation engine's,
  which they should not share.
- The `summary` rationale strings are server-formatted, so a client
  that wants different wording must ignore them. Acceptable until we
  see real client demand for templated rationale.
- Bucketing happens in Go, not SQL. For very long windows on very
  high-frequency signals (years of hourly buckets) the in-memory pass
  becomes wasteful. The 1000-bucket cap from D4 covers v1; when it
  bites we move to a SQL-side aggregation.

**Migration path.** None — new surface. No clients depend on a prior
shape. The package contract (`Store`, `Scorer`, `Signal`) is unchanged
from ADR-015; this server is purely additive.

---

## Validation

The server is correct when:

1. `tools/list` returns exactly three tools (`get_signals`,
   `get_recommendations`, `get_signal_trends`) with input schemas that
   round-trip through `json.Marshal`/`Unmarshal`.
2. `get_signals` against an empty store returns an empty array, not an
   error.
3. `get_signals{kind:"unknown"}` returns `-32602`; `get_signals` with a
   valid kind that has no rows returns an empty array.
4. `get_recommendations` returns the same top-N order as
   `aggregator.Scorer{Weights:...}.Score(...)` would for the same
   inputs (golden test against a deterministic in-memory clock).
5. `get_recommendations` reduces to *most recent per (kind, subject)*:
   inserting two `lint_trend` signals for the same subject in the
   window yields one recommendation row, with the *later* `Raw`.
6. `get_signal_trends{bucket:"24h", window:"72h"}` returns exactly 3
   buckets, including zero-count ones.
7. `get_signal_trends` rejects `window/bucket > 1000` with `-32602`.
8. The server starts cleanly when `signals.db` does not exist (the
   schema is applied on open) and `tools/list` succeeds against the
   empty file.

Phase 1 is done when those eight properties have tests, the binary
builds, and `go test ./... && golangci-lint run ./...` passes clean.

---

## References

- [ADR-015: Signal Aggregator](ADR-015-signal-aggregator.md) — data
  layer this server queries.
- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
  — JSON-RPC-over-stdio dispatch pattern.
- `cmd/dear-agent-mcp/workflow.go` — the dispatch scaffolding this
  server mirrors.
- `pkg/aggregator/scorer.go` — weighted-priority math reused verbatim.
