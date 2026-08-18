# ADR-009: Typed in-process event bus

Status: Accepted (2026-02-02; verified 2026-07-17)

## Context

Lifecycle, telemetry, plugin, and coordination consumers need to observe events
without each producer importing every downstream subsystem. Direct callbacks
create ownership cycles; a network broker would be excessive inside one
process.

## Decision

Use the typed local event bus as an in-process fan-out seam. Producers publish
domain events; subscribers register independently and must not own the durable
source of truth. Cross-process agent communication uses AGM bus/A2A surfaces,
not the local event bus.

## Alternatives

Direct calls tightly couple modules. A global untyped channel hides contracts.
Redis or NATS adds deployment state for an in-process need.

## Consequences

Handlers must remain bounded and observable, and delivery is process-local.
Event-bus and integration tests verify registration and dispatch.
