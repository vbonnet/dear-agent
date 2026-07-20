# ADR-001: Express Definition of Done checks in YAML

Status: Accepted

## Context

Completion criteria must be machine-readable while remaining reviewable beside a
Bead or task. The current check set is intentionally small: files, Go tests,
and commands with expected exit codes.

## Decision

The enforcer loads exactly one YAML document into `BeadDoD`. Known check
families remain typed; unknown future families are preserved as extension nodes
so older binaries can read newer files without claiming to execute those
checks. Implementing an extension still requires an explicit schema and
validator change.

## Consequences

- Criteria can be version-controlled and reviewed without compiling code.
- YAML parsing errors fail loading before any check runs.
- Schema evolution can add fields without breaking older readers, while new
  checks remain inactive until their validator ships.

## Evidence

- `../dod.go`
- `../dod_test.go`
