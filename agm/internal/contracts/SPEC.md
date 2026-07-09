# SLO Contracts Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/contracts` is the typed source for AGM lifecycle, scan, trust,
stall, audit, compaction, retry, health, alert, and daemon SLO configuration.

## Requirements

**SLO-01** When AGM loads contracts without an override, the system shall return the embedded maintained defaults.

**SLO-02** When a contract file is supplied, the system shall decode all supported sections and duration values into typed contracts.

**SLO-03** When a contract file is missing or invalid, the system shall return an explicit error instead of silently replacing it with defaults.

**SLO-04** When contracts are loaded repeatedly, the system shall reuse the cached immutable configuration until the test-only reset is invoked.

**SLO-05** When consumers request defaults directly, the system shall return complete non-zero safety thresholds for every supported contract section.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/contracts/*_test.go`
