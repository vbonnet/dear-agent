# Structured Logging Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/logging` constructs consistent text and JSON `slog` loggers for
normal and debug output.

## Requirements

**LOG-01** When the default logger is requested, the system shall return a usable structured logger with maintained output and level defaults.

**LOG-02** When JSON logging is requested, the system shall emit machine-readable JSON records.

**LOG-03** When debug logging is requested, the system shall enable debug-level records.

**LOG-04** When a text logger is created for a writer, the system shall send formatted records to that writer and enforce configured level filtering.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/logging/*_test.go`
