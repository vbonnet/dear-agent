# Debug Logger Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/debug` provides optional per-session debug logs and phase markers.

## Requirements

**DBG-01** When debug logging is disabled, the system shall avoid creating a log file and treat log calls as no-ops.

**DBG-02** When debug logging is enabled, the system shall initialize the session log destination and append formatted records.

**DBG-03** When a phase marker is recorded, the system shall emit a distinguishable phase boundary through the active logger.

**DBG-04** When the logger is absent or closed, the system shall allow repeated close and log calls without panicking.

**DBG-05** When debug logging is enabled, the system shall enforce owner-only permissions on the debug directory and log file, including when either path already exists with broader permissions.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/debug/*_test.go`
