# ADR-001: Verify real BUILD artifacts

<!-- Last audited at: 2026-07-17 -->

Status: Accepted
- **Scope:** Wayfinder BUILD completion
- **Decision owner:** Wayfinder validator

## Context

A BUILD phase can contain persuasive documentation without a working change.
Wayfinder must distinguish implementation evidence from prose while remaining
deterministic, bounded, and usable across supported languages.

## Decision

BUILD completion uses the active code-verification gate. It discovers supported
project manifests and source files inside the project boundary, rejects
placeholder-only deliverables, and runs applicable fixed build and test
commands with timeouts. Cached evidence is accepted only under the implemented
freshness and content checks.

The validator never creates synthetic test, review, deployment, or monitoring
success. Provider-backed reviews are supplemental and cannot bypass the
deterministic BUILD gate.

## Consequences

- BUILD claims are tied to inspectable files and command results.
- Unsupported project layouts fail with actionable evidence requirements
  instead of a fabricated pass.
- The gate may require explicit support for a new language or build layout.
- Tests must cover both successful verification and fail-closed behavior.

## Alternatives rejected

- Trusting `BUILD-*.md` alone: prose is not execution evidence.
- A synthetic BUILD state machine: it can report success without running the
  project.
- Arbitrary user-provided shell strings: they weaken path and argument safety.

## Verification

```sh
go test ./wayfinder/cmd/wayfinder-session/internal/validator
```
