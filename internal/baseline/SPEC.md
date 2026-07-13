# Benchmark Baseline Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/baseline`.

## Overview

`internal/baseline` persists versioned benchmark measurements, regression
thresholds, git provenance, and update history. Its data contract is independent
of the harness or model family whose workflow produced the measurement.

## EARS Requirements

**BASELINE-01** When a baseline is loaded, the system shall reject malformed JSON, unsupported schema versions, and invalid scenario fields.

**BASELINE-02** When a baseline is saved, the system shall validate the complete schema before replacing persisted data.

**BASELINE-03** When no baseline exists during an update, the system shall create a baseline with the current schema and default thresholds.

**BASELINE-04** When an existing scenario is updated, the system shall retain the previous median and current git provenance in scenario history.

**BASELINE-05** When scenario names are requested, the system shall return every persisted scenario name without changing the baseline.

**BASELINE-06** When thresholds are validated, the system shall reject a local multiplier at or below one and percentages outside their supported ranges.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/baseline`
