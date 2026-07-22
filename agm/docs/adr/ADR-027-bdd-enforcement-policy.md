# ADR-027: Go-native BDD enforcement

Status: Accepted (verified 2026-07-17)

## Context

Behavior specifications need to run in the normal Go toolchain and remain
traceable to strict subsystem specifications. A separate build system would add
another dependency graph and local/CI parity problem.

## Decision

AGM BDD uses Go tests and the repository feature suite. Specifications use
stable requirement IDs; feature files link to their owning SPEC and source, and
reciprocal-link checks reject orphaned coverage. CI invokes the Go test package
directly.

Every feature lives under `agm/test/bdd/features/` and runs without an
`@implemented` or similar selection filter. Discovery and execution therefore
cover the complete tracked feature catalog rather than an opt-in subset.

## Alternatives

Bazel adds substantial tooling for a Go monorepo already built by `go test`.
Unlinked Gherkin scenarios look complete while drifting from requirements.
Unit tests alone do not provide the cross-package behavior catalog.

## Consequences

The feature suite is large and requires disciplined ownership links. Strict SPEC
lint and BDD tests verify the policy.
