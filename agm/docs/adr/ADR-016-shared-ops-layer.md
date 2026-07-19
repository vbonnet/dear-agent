# ADR-016: Shared operations layer

Status: Accepted (2026-03-23; amended 2026-07-17)

## Context

CLI, MCP, skills, background jobs, and reapers once reimplemented lifecycle
mutations with different validation, storage, and cleanup behavior.

## Decision

`agm/internal/ops` owns typed state-changing operations and stable structured
errors. CLI and MCP are adapters; skills invoke a structured CLI surface.
Session creation and archival each have one ordered operation with dependency
ports on `OpContext`, caller provenance, rollback, and harness-specific edge
adapters. Bulk selection and asynchronous reaping may prepare or aggregate work
but do not duplicate the durable transition.

Claude UI archival is a separate namespace under ADR-026 because it reconciles
provider records rather than AGM session lifecycle.

## Alternatives

CLI-only business logic leaves MCP divergent. A mandatory service layer would
make the local CLI depend on a daemon. Shared untyped helpers do not define
transition ownership.

## Consequences

All surfaces share guards and outcomes, while adapters retain presentation and
interaction differences. Operations, CLI, MCP, reaper, and archive tests verify
the transition order.
