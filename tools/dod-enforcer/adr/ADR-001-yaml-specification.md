# ADR-001: Express Definition of Done checks in YAML

Status: Accepted

## Context

Completion criteria must be machine-readable while remaining reviewable beside a
Bead or task. The current check set is intentionally small: files, Go tests,
and commands with expected exit codes.

## Decision

The enforcer loads a typed YAML document into `BeadDoD`. Unknown future check
families require an explicit schema and implementation change; comments in the
Go type are not accepted configuration.

## Consequences

- Criteria can be version-controlled and reviewed without compiling code.
- YAML parsing errors fail loading before any check runs.
- Schema evolution must preserve or migrate existing files deliberately.

## Evidence

- `../dod.go`
- `../dod_test.go`
