# ADR-006: Fail closed when tests target production AGM state

Status: Accepted

## Context

AGM tests exercise storage and session lifecycles that can delete or mutate
records. Environment leakage once allowed tests to reach a real workspace.
Naming conventions and cleanup after the fact are not sufficient isolation.

## Decision

Test helpers select an explicit test workspace and database. Storage adapters
reject known test execution when the configured target is production-like.
Integration tests provision their own state and clean only resources they
created. Host-dependent tests must opt in and identify their boundary.

## Consequences

- Misconfigured tests fail before mutating production state.
- Test setup is more explicit.
- Host integration coverage remains separate from hermetic package tests.

## Evidence

- `../../internal/dolt/adapter.go`
- `../../internal/testutil/` and `../../internal/testcontext/`
- test-isolation scenarios in `../../test/bdd`
