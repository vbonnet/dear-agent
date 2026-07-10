# Log Store Contract Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/logs` defines the harness-neutral storage contract and record
shape for querying structured AGM logs.

## Requirements

**LGS-01** When a log entry is stored, the system shall preserve its timestamp, level, message, session identity, component, and optional structured data.

**LGS-02** When log entries are listed, the system shall support maintained session, level, component, limit, and time filters through `ListOpts`.

**LGS-03** When optional structured data is absent, the system shall permit a nil data payload without changing the record contract.

**LGS-04** When a store implementation is provided, the system shall expose append and filtered-list operations through the shared interface.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/logs/*_test.go`
