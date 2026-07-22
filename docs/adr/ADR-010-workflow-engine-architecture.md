# ADR-010: Durable workflow execution substrate

Status: Accepted (2026-05-02; verified 2026-07-17)

## Context

A session is a process container, not a durable work item. The original
workflow runner stored lossy JSON snapshots, could not answer concurrent status
queries, and did not preserve every transition. A separate ADR-009 described
the same substrate need without owning an implementation boundary.

## Decision

`pkg/workflow` owns durable workflow execution:

- YAML defines dependency-ordered nodes; a run and each node have explicit
  lifecycle states.
- SQLite `runs.db` is the default state store, with an audit event for every
  transition and first-class HITL requests.
- AI nodes express roles and bounded execution policy; adapters resolve roles
  to providers outside the workflow definition.
- CLI, gateway, and bus surfaces call the package rather than owning parallel
  state machines.

The former ADR-009 distinction is absorbed here: work-item state is durable
workflow data and remains separate from AGM session state.

## Alternatives

JSON snapshots are simple but not safely queryable mid-run. Reusing AGM session
manifests conflates execution with process lifecycle. A hosted workflow service
would add an operational dependency for a local-first system.

## Consequences

SQLite write concurrency is a deliberate local-scale limit. Durable state makes
resume, audit, HITL, and external status surfaces consistent. Package tests and
the `cmd/workflow-*` commands verify the boundary.
