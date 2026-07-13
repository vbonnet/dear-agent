# Command Benchmark Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/benchmark`.

## Overview

`internal/benchmark` measures a caller-selected command against deterministic
fixture scenarios and summarizes successful timings. It records failures
without allowing one failed iteration to erase successful measurements.

## EARS Requirements

**BENCH-01** When an executor is created, the system shall configure bounded runs, warmup runs, a fixture scenario, and a per-run timeout.

**BENCH-02** When a benchmark starts, the system shall stage only the fixtures for the selected scenario and unstage them after execution.

**BENCH-03** When a command exceeds its per-run timeout, the system shall record a timeout failure for that iteration.

**BENCH-04** When measured iterations succeed, the system shall calculate median, mean, standard deviation, percentile, and coefficient-of-variation statistics from successful timings.

**BENCH-05** When an iteration fails, the system shall preserve its diagnostic and continue the configured benchmark run.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/benchmark`
