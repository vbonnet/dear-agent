# ADR-015: Signal aggregator and recommendation MCP

Status: Accepted (2026-05-03; verified 2026-07-17)

## Context

Git, lint, coverage, dependency, and security findings use incompatible shapes.
A recommendation surface needs durable heterogeneous inputs, time-range
queries, and tunable ranking without coupling collection to presentation.
`pkg/signals` already names a different progressive-rigor concept.

## Decision

- `pkg/aggregator` owns the `Signal` model, independent collectors, SQLite
  `signals.db`, and weighted scoring.
- Collector failure is isolated in the run report so one unavailable tool does
  not erase other signals.
- `cmd/dear-agent-signals` is the collection/report CLI.
- `cmd/recommendation-mcp` is a read-only MCP surface over the same store. It
  exposes summaries, recommendations, and subject history; it never runs
  collectors or mutates policy.

This record absorbs the former ADR-016 recommendation-server record.

## Alternatives

One table per source hard-codes current collectors. A live-only MCP server
cannot answer trends. Reusing `pkg/signals` would merge unrelated domains.

## Consequences

Operators own weights and collection cadence. SQLite is a local-scale boundary.
Collector, store, scorer, CLI, and MCP tests verify the decision.
