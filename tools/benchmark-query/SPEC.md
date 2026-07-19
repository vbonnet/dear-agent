# benchmark-query — Specification

<!-- Last audited at: NEEDS-AUDIT -->

## Executable EARS Requirements

**BQR-01** When benchmark records are queried, the tool shall apply configured filters and return deterministic ordered results.

**BQR-02** If benchmark input is missing or invalid, then the tool shall return an actionable error without fabricating measurements.

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

## Overview

benchmark-query provides a CLI for querying benchmark metrics collected from
ai-tools test runs and session infrastructure. It reads metric files from a
local directory and supports filtering by time window.

## Functional Requirements

- Query individual metrics by name
- List all available metrics
- Filter results by time window (`-since`)
- Output in human-readable or JSON format
- Read metrics from configurable directory

## Non-Functional Requirements

- Query latency: < 100ms for local metric files
- No external dependencies (reads local files only)
- Exit code 0 on success, non-zero on error
