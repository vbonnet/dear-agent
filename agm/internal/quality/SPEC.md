# Quality Debt Baseline Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/quality` measures Go vet findings against a persisted quality-debt
baseline so new regressions fail without requiring an immediate backlog burn-down.

## Requirements

**QLT-01** When Go vet is run, the system shall capture non-empty finding lines and return execution failures.

**QLT-02** When a baseline is saved, the system shall persist its issue count and assign a timestamp when absent.

**QLT-03** When a baseline file is missing or invalid, the system shall return an explicit load error.

**QLT-04** When the current issue count exceeds the baseline, the system shall report a quality regression.

**QLT-05** When the current issue count is equal to or below the baseline, the system shall accept the result.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/quality/*_test.go`
