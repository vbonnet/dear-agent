# Scheduled Sandbox-GC Log Health Specification

## Purpose

`internal/gcloghealth` is the one observation boundary used by
`cmd/disk-watchdog` and `cmd/sweep-health` to decide what an append-only
`gc.jsonl` log proves about the scheduled sandbox reaper. It returns evidence,
not a command status, exit code, alarm, or remediation decision. The producer
wire type remains in `agm/internal/gclog`, which root commands cannot import;
both sides pin one literal JSONL fixture.

## EARS Requirements

**GCLH-01** When either health command reads sandbox-GC JSONL, the observer shall decode and fold that log through one shared contract.

**GCLH-02** When a record has the ignored `disk-watchdog` source, the observer shall not accept it as scheduled-reaper success, reap fallback, or diagnostic error.

**GCLH-03** When a completion is dry-run or reports deletion errors or safety-probe failures, the observer shall reject it as proof and retain the reason for the rejection.

**GCLH-04** When a sandbox-GC error record is newer than the latest accepted proof, the observer shall retain its explicit error text while ignoring unrelated session-GC errors.

**GCLH-05** When a record is more than five minutes ahead of evaluation time, the observer shall not accept it as liveness proof and shall retain future-completion evidence for the sweep-health command's DOWN policy.

**GCLH-06** When the initial whole-record tail has no valid proof but recent unscanned history might contain one, the observer shall widen the scan up to a configured hard byte cap.

**GCLH-07** When a widened scan reaches the hard cap without resolving recent unscanned history, the observer shall mark the result indeterminate instead of declaring that no sweep ever happened.

**GCLH-08** When a record is malformed or oversized, the observer shall skip that record and continue with later whole records; when reading or seeking fails, the observer shall return the I/O error.

**GCLH-09** When a complete log has an untagged reap and no completion record of any source, the observer shall offer that reap as legacy proof.

**GCLH-10** When a log is truncated or contains any modern completion record, the observer shall not offer an untagged reap as legacy proof.

**GCLH-11** When a reap carries a nonempty modern source tag, the observer shall not offer that reap as legacy proof even if no completion follows.

**GCLH-12** When the watchdog's own modern completion is present beside a historical untagged reap, the observer shall use that completion only to disqualify legacy fallback, never as scheduled-reaper success.

## Traceability

- `scan_test.go` exercises source filtering, rejection reasons, explicit
  errors, future timestamps, widening, indeterminate history, legacy proof,
  historical mixed logs, malformed records, and the shared producer fixture.
- `agm/internal/gclog/gclog_test.go` verifies the producer's wire fields
  against `testdata/wire.jsonl` without an illegal `agm/internal` import.
- `agm/internal/ops/sandbox_gc_test.go` verifies that the per-sandbox reap
  producer carries its declared runner source under a synthetic HOME.
- Command-specific mapping remains in `cmd/disk-watchdog/SPEC.md` and
  `cmd/sweep-health/SPEC.md` with adapter tests in those packages.
