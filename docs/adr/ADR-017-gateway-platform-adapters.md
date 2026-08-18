# ADR-017: Transport-neutral gateway handlers

Status: Accepted (2026-05-03; verified 2026-07-17)

## Context

CLI, HTTP, MCP, and chat surfaces otherwise repeat request validation, identity
handling, workflow dispatch, and response shaping. A network message broker
would be excessive for the in-process seam.

## Decision

`pkg/gateway` owns transport-neutral commands, responses, events, and handlers
over `pkg/workflow`. Adapters translate their transport at the edge and pass
caller identity through unchanged. The gateway stores no workflow state; it
uses the workflow database and publishes events to interested adapters.

The HTTP adapter composes with `pkg/api`. AGM integration lives outside the
root package so root modules do not import AGM internals.

## Alternatives

Per-transport handlers drift. NATS or Redis would add deployment state to a
local in-process boundary. Moving workflow logic into the gateway would create
a second execution engine.

## Consequences

Wire envelopes remain dynamically shaped at the transport seam, while domain
responses remain typed. Adapter and handler tests under `pkg/gateway` verify
identity propagation and command behavior.
