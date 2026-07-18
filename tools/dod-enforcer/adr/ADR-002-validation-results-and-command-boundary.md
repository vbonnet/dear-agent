# ADR-002: Separate validation results from command execution errors

Status: Accepted

## Context

A required file missing or a command returning an unexpected exit code is a
normal failed check. Failure to start a process or exceeding its time boundary
is an execution error with different diagnostic value.

## Decision

Validation returns a structured result containing every check outcome and an
overall success value. Configured command checks run through a bounded shell
boundary because the configuration intentionally stores command strings. The
result records expected versus actual exit status and output; infrastructure
errors and timeouts remain explicit failures.

Checks run in deterministic declaration order. Parallelism is not part of the
contract.

## Consequences

- Callers can report all completed evidence instead of parsing one error.
- Command strings carry shell semantics and must come from trusted task
  configuration.
- Time limits are implementation policy and may evolve without a new ADR.

## Evidence

- `../dod.go`
- command and validation tests in `../dod_test.go`
