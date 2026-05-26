# ADR-015: Signal Aggregator + Recommendation MCP

**Status**: Proposed (2026-05-03; recommendation-MCP section absorbed
from [ADR-016](ADR-016-recommendation-mcp-server.md) on 2026-05-26).

A recommendation engine that says "fix lint, then refresh deps, then
add coverage to `pkg/foo`" needs three things the codebase doesn't have
today: heterogeneous inputs in one place (git/lint/coverage/deps/security
live in five tools with five output formats); a persistent time-series
store ("is lint going up this week?"); and a weighted scorer so
operators can tune what dominates.

`pkg/aggregator/` is that data layer. `cmd/recommendation-mcp/` is the
MCP-native read surface on top of it. Both are described here; ADR-016
is a redirect for inbound code references.

## Part A — Signal Aggregator (`pkg/aggregator/`)

`Collector` interface + SQLite-backed `Store` + an `Aggregator` runner
+ a `Scorer` for weighted priority. Five Phase 1 collectors ship as
first-party: `dear-agent.git`, `.lint`, `.coverage`, `.deps`, `.security`.
A new binary `cmd/dear-agent-signals` exposes `collect` and `report`.

- **D1. Package name is `pkg/aggregator/`, not `pkg/signals/`.**
  `pkg/signals/` already exists with a different meaning (Hybrid
  Progressive Rigor signals — keyword/effort/file-type fusion). The
  exported type is `Signal`; the domain noun lives in the package path,
  not the type name. The collision is documented in CONTEXT.md.
- **D2. `Signal` is `{ID, Kind, Subject, Value, Metadata, CollectedAt}`.**
  `Subject` is intentionally free-form (repo path, Go import path,
  module path, vuln ID). The schema does not model cross-kind
  relationships — ranking is per-kind first, weighted-sum second.
  `Metadata` is JSON in a TEXT column, matching `pkg/workflow`'s
  `inputs_json` / `audit_events.payload_json`.
- **D3. Collectors are independent of the store.** Each takes an `Exec`
  indirection (`func(ctx, name, args ...) ([]byte, error)`) so unit
  tests fake out external commands; production wires `exec.CommandContext`.
  `Name()` follows the `dear-agent.<area>` convention from
  [ADR-014 §D2](ADR-014-plugin-system.md). The
  [ADR-014](ADR-014-plugin-system.md) `SignalCollector` capability
  reservation slots in additively.
- **D4. One table, two indexes.** `signals(signal_id, kind, subject,
  value, metadata_json, collected_at)`; `(kind, collected_at)` covers
  `Recent` / `Range`; `subject` covers per-package drill-down. Driver
  is `modernc.org/sqlite` with the same `busy_timeout(5000) +
  journal_mode(WAL) + foreign_keys(on)` triple as `pkg/workflow`.
- **D5. A failing collector does not fail the run.** Errors land in the
  per-run `Report`; other collectors continue. Same "audit emission is
  unconditional" guarantee as
  [ADR-010 §D3](ADR-010-workflow-engine-architecture.md). Concurrency
  is sequential in v1; the interface does not preclude fan-out.
- **D6. The `Scorer` reduces to most-recent-per-(kind, subject), then
  weights.** Internal clamping ceilings (lint at 200, coverage inverted
  against 100, deps at 50, security at 10, git at 100 commits/week)
  are constants in v1 — operator-tunable when there are real signals
  to look at. `DefaultWeights`: security dominates (1.0), coverage 0.5,
  lint 0.4, deps 0.3, git 0.2.
- **D7. Five Phase 1 collectors** wrap `git log`, `golangci-lint run
  --output.json.path=stdout`, `go test -cover ./...` (or a
  `--coverage-file`), `go list -u -m -json all`, `govulncheck -json
  ./...`. Missing tools surface as `ErrToolMissing{Tool: ...}` rather
  than panicking; the report names them.

A **second SQLite file** (`signals.db`) is deliberate. `runs.db` is the
work-item store; consolidating would muddle backup, retention, and
cadence. The default path is `./.dear-agent/signals.db` if the dir
exists, else `./signals.db`.

## Part B — Recommendation MCP (`cmd/recommendation-mcp/`)

Aggregator data is queryable by Go and by `dear-agent-signals report`.
Neither is useful to an MCP client. Add a read-only MCP server that
exposes the aggregator over stdio so any client (Claude Code, Cursor,
custom agents) can ask "what should we work on next?" without knowing
the SQLite schema.

Three tools, all read-only. Dispatch shape and error codes are copied
from `cmd/dear-agent-mcp` so future MCP-wide changes (auth, tracing,
batch) land in both servers via the same edit.

- **D-MCP-1. `get_signals`** — filtered query: optional `kind`,
  optional `subject` (substring `LIKE '%' || ? || '%'`), optional
  `since` (RFC3339), `limit` capped at 1000. Substring matching is
  what the free-form `Subject` convention wants; bulk export bypasses
  this and queries SQLite directly.
- **D-MCP-2. `get_recommendations`** — top-N weighted scores.
  Algorithm: pull every known kind, fetch within `window` (default
  `168h`), reduce to *most recent per `(kind, subject)`*, run
  `Scorer.Score`, truncate to `top_n` (default 10, max 50). `weights`
  overrides go into `Scorer.Weights`; missing keys fall back to
  `DefaultWeights`. A server-formatted `summary` string per row
  ("<kind> on <subject> (raw=<value>)") lets clients render without
  owning the prose.
- **D-MCP-3. `get_signal_trends`** — time-bucketed aggregation. Bucket
  math in Go (cheap; one pass; lets the server use the caller's
  zero-point without growing SQL dialect surface). Min bucket 1h;
  `window/bucket > 1000` returns `-32602`. Empty buckets are emitted
  so clients can distinguish "no signal" from "lost collection".

A separate binary, not new tools on `cmd/dear-agent-mcp`. Write paths
(`runs.db`, `sources.db`) and read-only signal queries do not share a
release cadence and may live on different hosts (CI fleet vs. developer
laptop). Read-only is enforced at the boundary by *not exposing* a
write method; the open path also uses `?mode=ro` as defense in depth.

`.mcp.json` entry name is `dear-agent-recommendations` (the
`dear-agent.<area>` namespace from
[ADR-014 §D8](ADR-014-plugin-system.md)); the binary on disk is what
`PATH` resolves.

### Out of scope (both parts)

- The full recommendation algorithm beyond "top weighted scores".
- Streaming / push collection. Phase 1 is poll-only.
- Per-author signals. Author attribution needs its own
  privacy/permissions conversation.
- Cross-repo aggregation. One signals.db, one repo. Fleet view is a
  separate tool that merges per-repo files.
- Authentication on the MCP server. Stdio is local-process; HTTP is
  dev-only. A production deployment gets its own auth ADR.

### Cross-references

- [ADR-010](ADR-010-workflow-engine-architecture.md) — SQLite substrate
  pattern, JSON-RPC-over-stdio dispatch.
- [ADR-014](ADR-014-plugin-system.md) — `SignalCollector` capability
  reservation.
- `pkg/workflow/state_sqlite.go` — driver, pragmas, busy-retry pattern
  this ADR mirrors.
- `cmd/dear-agent-mcp/workflow.go` — dispatch scaffolding the
  recommendation MCP mirrors.
