# ADR-002: Separate validation results from command execution errors

Status: Accepted

## Context

A required file missing or a command returning an unexpected exit code is a
normal failed check. Failure to start a process or exceeding its time boundary
is an execution error with different diagnostic value.

## Decision

Validation returns a structured result containing every check's type, name,
success value, and optional error text plus an overall success value.
Configured command checks run through a bounded shell boundary because the
configuration intentionally stores command strings. On mismatch, expected and
actual exit status and output are embedded in the check's error text;
successful command output is not retained as structured evidence.

Checks run in deterministic declaration order. Parallelism is not part of the
contract.

## Consequences

- Callers can enumerate completed checks, but must treat command diagnostics as
  prose rather than a stable structured schema.
- Command strings carry shell semantics and must come from trusted task
  configuration.
- Time limits are implementation policy and may evolve without a new ADR.

## Evidence

- `../dod.go`
- command and validation tests in `../dod_test.go`
