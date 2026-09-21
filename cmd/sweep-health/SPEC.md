# Sweep Health Specification

<!-- Last audited at: 2026-09-05 -->

## Purpose

`cmd/sweep-health` reports whether the sandbox garbage collector has produced its
positive event: at least one proof-of-completed-sweep record within a configured
lookback window (default 6h, matching DW-17). It is a sibling of `cmd/bead-health`,
`cmd/merge-health`, and `cmd/jaeger-health`, conforming to the shared absence-alarm
probe interface: exit 0 healthy, 1 degraded, 2 down, 3 usage, with a `--json` report
and a `--lookback` window.

Disk free space is a lagging indicator of sandbox leakage; a reaper that has stopped
completing sweeps is the leading indicator. This probe evaluates sandbox GC records
against the strict DW-21 criteria: a non-dry-run completion record with zero reap
errors and zero probe failures.

The probe is strictly read-only: it inspects `gc.jsonl` and never triggers remediation,
reaping, or file modifications.

## Shared absence-probe contract

The status vocabulary, exit codes, and JSON envelope in SWEEP-01..SWEEP-09 implement
the generic absence-alarm probe interface defined in `pkg/absencealarm/SPEC.md`.

## EARS Requirements

**SWEEP-01** When the sandbox GC log contains at least one valid proof-of-completed-sweep record within the lookback window, the sweep-health probe shall report healthy and exit 0.

**SWEEP-02** When the sandbox GC log is accessible but contains no proof-of-completed-sweep record within the lookback window, the sweep-health probe shall report degraded with the latest sweep timestamp and age and exit 1.

**SWEEP-03** When the sandbox GC log is accessible but contains zero completed sweep records, the sweep-health probe shall report degraded and exit 1.

**SWEEP-04** When the sandbox GC log cannot be read or does not exist, the sweep-health probe shall report down with the underlying error and exit 2.

**SWEEP-05** When the latest completed sweep record timestamp is more than the clock-skew tolerance (5 minutes) in the future, the sweep-health probe shall report down rather than healthy and exit 2.

**SWEEP-06** When the sweep-health lookback window cannot be parsed or is not positive, the sweep-health probe shall report a usage error and exit 3.

**SWEEP-07** When JSON output mode is set, the sweep-health probe shall emit a single JSON report carrying status, log path, lookback window, latest completed sweep timestamp, age, and any error message.

**SWEEP-08** When evaluating proof of a completed sweep, the sweep-health probe shall accept only a non-dry-run completion record with zero reap errors and zero probe failures.

**SWEEP-09** When scanning the sandbox GC log, the sweep-health probe shall begin with a bounded whole-record tail and widen only while unscanned history could contain live proof, up to a hard byte cap, so a fixed tail cannot falsely claim a recent scheduled sweep never happened.

**SWEEP-10** When a sandbox GC record has `source=disk-watchdog`, the sweep-health probe shall exclude that record from liveness proof, legacy reap fallback, and error diagnosis, so watchdog remediation cannot certify the scheduled reaper.

**SWEEP-11** When an explicit sandbox-GC error follows the latest accepted proof, the sweep-health probe shall include that error in a degraded report while ignoring unrelated session-GC errors.

**SWEEP-12** When the hard scan cap leaves recent unscanned history that could contain a heartbeat, the sweep-health probe shall report degraded and undetermined rather than healthy or a definitive never-swept claim.

**SWEEP-13** When a reap record is the only apparent proof, the sweep-health probe shall accept legacy fallback only for an untagged reap after a complete scan establishes that the log contains no modern completion record; at the hard cap it shall report undetermined.

## BDD Traceability

- Feature: `agm/test/bdd/features/observability_package_guardrails.feature`
- Package tests: `cmd/sweep-health/main_test.go` (report/exit mapping and
  SWEEP-10 through SWEEP-12) and `internal/gcloghealth/scan_test.go`
  (shared admission, widening, hard cap, and SWEEP-13 fallback proof).
