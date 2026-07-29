# ADR-001: CLI harness adapter boundary

Status: Accepted (2026-01-15; amended 2026-07-17 and 2026-07-27)

## Context

AGM must manage several interactive coding harnesses without making one
harness's command vocabulary the session model. The original record, command
translation ADR-002, and Gemini-first ADR-011 repeated the same adapter choice
while mixing API providers with CLI harnesses.

## Decision

Concrete CLI adapters own harness-specific launch, resume, message, output,
availability, and capability differences. `activeHarnesses` and
`deprecatedHarnesses` in `harnesses.go` are the finite inventory. Shared
lifecycle and storage live outside adapters in operations and manager modules.
Heterogeneous discovery uses the metadata-only `agent.Harness` contract;
behavioral interfaces are owned by their consumers as decided in ADR-031.

Unsupported capabilities return explicit errors; AGM does not emulate them by
silently calling a different harness or API provider. This record absorbs the
former ADR-002 and ADR-011.

## Alternatives

One binary per harness duplicates lifecycle and storage. A monolithic switch
spreads harness conditionals through every command. API-provider adapters do not
represent interactive CLI session behavior.

## Consequences

New harnesses implement the metadata contract and join the finite constructor
catalog. Shared behavior must remain provider-neutral, while richer harness
extensions stay concrete or behind a consumer-owned capability interface.
Harness conformance, operation tests, and adapter tests verify the boundaries.
