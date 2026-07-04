# A2A Metrics Collector Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/metrics` records per-channel coordination metrics, including
token usage, response intervals, participant stats, consensus data, and status
transitions.

## EARS Requirements

**A2A-MET-01** When a metrics collector is created, the system shall require a non-empty channels directory and normalize an `active` directory to its parent.

**A2A-MET-02** When metrics are initialized for a channel, the system shall create `metrics.json` with schema version, timestamps, empty metric collections, and `0600` permissions.

**A2A-MET-03** When metrics already exist for a channel, the system shall reject reinitialization instead of overwriting existing metrics.

**A2A-MET-04** When recording a message, the system shall update totals, average tokens, min/max tokens, per-message records, response intervals, participants, and budget violation count.

**A2A-MET-05** When recording status changes or consensus signals, the system shall append transition and consensus data without losing prior metric history.

**A2A-MET-06** When updating metrics, the system shall use file locking so concurrent writers do not corrupt `metrics.json`.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/metrics/collector_test.go`
