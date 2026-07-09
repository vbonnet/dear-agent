# Benchmark Evaluation Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/benchmark` parses Go benchmark output and evaluates measured
operations against named performance targets.

## Requirements

**BENCH-01** When benchmark output contains valid result lines, the system shall parse operation counts, latency, allocation bytes, and allocation counts when present.

**BENCH-02** When benchmark output reports a failed run or no valid benchmarks, the system shall return an explicit parse failure.

**BENCH-03** When evaluating benchmark results, the system shall compare each named metric with its configured target and report pass and failure details.

**BENCH-04** When no custom targets are supplied, the system shall expose the maintained default performance targets.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/benchmark/*_test.go`
