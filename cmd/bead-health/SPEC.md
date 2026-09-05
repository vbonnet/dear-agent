# Bead Health Specification

<!-- Last audited at: 2026-09-05 -->

## Purpose

`cmd/bead-health` reports whether the Beads project store has produced its
positive event: at least one bead closed within a configured lookback window.
It is a sibling of `cmd/merge-health` and `cmd/jaeger-health`, deliberately
copying their contract: a standalone, no-side-effect probe with the shared
exit-code contract 0 healthy / 1 degraded / 2 down / 3 usage, a `--json`
report, and a `--lookback` window - so a generic scheduler-and-sink layer
(`cmd/absence-alarm`) can register it as a command pulse without bespoke logic.

Work in this repository is tracked in Beads. When the pipeline stalls, agents
stop closing beads. This probe detects that silence and provides immediate
visibility into recent closure age and identity.

The probe is read-only by design. It never mutates the database, never creates,
updates, or closes beads, and passes `--readonly` to all store queries (BH-08).

## Shared absence-probe contract

The status vocabulary, exit codes, and JSON envelope in BH-01..BH-09 are not
bead-specific: they are the generic absence-probe interface a scheduler
consumes. `pkg/absencealarm/SPEC.md` and `cmd/jaeger-health/SPEC.md` define
that shared exit contract: exit 0 healthy, 1 degraded, 2 down, 3 usage, and
a single JSON report shape.

## EARS Requirements

**BH-01** When at least one closed bead exists in the database with a closure time inside the lookback window, the system shall report healthy and exit 0.

**BH-02** When the database is accessible but no closed bead has a closure time inside the lookback window, the system shall report degraded with the latest closed bead ID, title, and age and exit 1.

**BH-03** When the database is accessible but contains zero closed beads, the system shall report degraded and exit 1.

**BH-04** When the database cannot be read or the query tool fails, the system shall report down with the underlying error and exit 2.

**BH-05** When the latest closed bead closure time is more than the clock-skew tolerance (5 minutes) in the future, the system shall report down rather than healthy and exit 2.

**BH-06** When the bead-health lookback window cannot be parsed or is not positive, the system shall report a usage error and exit 3.

**BH-07** When JSON output mode is set, the system shall emit a single JSON report carrying status, database path, lookback window, latest closed bead ID, title, closure time, age, and any error message.

**BH-08** The system shall not mutate the database: it shall run all queries in read-only mode and execute no write operations of any kind.

**BH-09** When the probe runs any subprocess, the system shall bound execution with a single tick deadline so that a hung invocation reports a bounded failure instead of stalling the scheduler that invoked it.

## BDD Traceability

- Feature: `agm/test/bdd/features/observability_package_guardrails.feature`
- Package tests: `cmd/bead-health/main_test.go`
