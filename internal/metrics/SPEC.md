# Internal Metrics Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/metrics` records benchmark and health metrics as JSONL so AGM and
workflow retrospectives can measure test pass deltas, false completion claims,
hook bypasses, and session outcomes without requiring an external metrics
service.

## Requirements

**METRICS-01** When a metrics store is created without an explicit directory, the system shall use `~/.agm/benchmarks` and create it with owner-only permissions.

**METRICS-02** When a metrics record is appended without a timestamp, the system shall set the timestamp to the current UTC time before writing JSONL.

**METRICS-03** When metrics are queried and the metrics file does not exist, the system shall return an empty result set without error.

**METRICS-04** When metrics are queried, the system shall skip malformed JSONL rows and return records matching metric, since-time, and until-time filters.

**METRICS-05** When a metric summary is requested for an empty result set, the system shall return no summary.

**METRICS-06** When a metric summary is requested for matching records, the system shall compute count, mean, latest, minimum, maximum, category, and metric name.

**METRICS-07** When test pass rate is recorded, the system shall persist the after-rate minus before-rate as an outcome-quality metric with before and after labels.

**METRICS-08** When completion validity is recorded, the system shall persist `1.0` for claimed-done work without passing tests or commits and `0.0` otherwise.

**METRICS-09** When hook events are recorded, the system shall persist hook bypass rate as a health metric with hook, pattern, and blocked labels.

**METRICS-10** When session outcomes are recorded, the system shall persist `1.0` only for `completed` outcomes and `0.0` for failed or abandoned outcomes.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
