# Benchmark Common Utilities Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/common`.

## Overview

`internal/common` supplies benchmark fixture staging, git provenance, and
deterministic statistical helpers shared by baseline and benchmark commands.

## EARS Requirements

**COMMON-01** When git provenance is requested inside a repository, the system shall return the current commit and branch identifiers.

**COMMON-02** When git provenance is requested outside a repository, the system shall return a diagnostic instead of fabricated metadata.

**COMMON-03** When benchmark fixtures are staged, the system shall create and stage only the files defined by the selected scenario.

**COMMON-04** When benchmark fixture cleanup runs, the system shall remove staged synthetic files without retaining benchmark state.

**COMMON-05** When statistical helpers receive a sample, the system shall calculate median, mean, standard deviation, and percentile values deterministically.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/common`
