# ADR-013: Persist launch permission policy

Status: Accepted (verified 2026-07-17)

## Context

Permission mode affects the safety contract of a harness session. Applying it
only to the first launch makes resume behavior depend on hidden shell history.
Not every harness expresses permission policy with the same flags.

## Decision

AGM stores normalized permission intent with session lifecycle data and passes
it through the shared harness launch builder. Each adapter translates supported
intent to its own launch contract or rejects unsupported modes explicitly.
Resume and fresh-start fallback use the same policy source.

## Alternatives

Remembering only the rendered command is brittle across harness versions.
Defaulting on every resume can silently weaken or strengthen the original
session policy.

## Consequences

Permission parity depends on adapter support and conformance tests. Shared
launch, model, and session-creation tests verify persistence.
