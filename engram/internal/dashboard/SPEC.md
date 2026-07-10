# Engram Analytics Dashboard Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/dashboard` queries telemetry-backed quality metrics and
formats them for human and machine consumption.

## EARS Requirements

**EDB-01** When dashboard display is requested, the system shall validate the metric name before opening telemetry storage.

**EDB-02** When telemetry storage is missing, inaccessible, or empty, the system shall return an actionable data-availability error.

**EDB-03** When date bounds are supplied, the system shall apply them consistently to specificity, example, token-efficiency, and trend queries.

**EDB-04** When specificity metrics are queried, the system shall group outcomes by specificity bucket and calculate success rates from total and successful launches.

**EDB-05** When example metrics are queried, the system shall compare launches with and without examples without discarding empty result sets.

**EDB-06** When token-efficiency and trend metrics are queried, the system shall preserve aggregate token, quality, cost, and time dimensions from telemetry.

**EDB-07** When dashboard results are formatted, the system shall support table or Markdown, CSV, and JSON output with stable fields.

**EDB-08** When all metrics are selected, the system shall render every metric group using the requested time range and format.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_observability_guardrails.feature`
- Package tests: `engram/internal/dashboard/*_test.go`
