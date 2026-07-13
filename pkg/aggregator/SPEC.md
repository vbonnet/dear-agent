# Project Health Aggregator Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/aggregator`.

## Overview

`pkg/aggregator` collects, validates, scores, and persists project-health
signals. It tolerates individual collector failures while keeping storage
failures explicit.

## EARS Requirements

**AGG-01** When an aggregation run has no store, the system shall reject the run before invoking collectors.

**AGG-02** When a collector succeeds, the system shall stamp missing identifiers and timestamps and persist signals matching the collector kind.

**AGG-03** When a collector emits a mismatched kind, the system shall exclude that signal without poisoning valid collector output.

**AGG-04** When a collector fails, the system shall retain its diagnostic and continue running the remaining collectors.

**AGG-05** When the store insert fails, the system shall fail the run with the storage diagnostic and completed run timestamps.

**AGG-06** When a signal reaches the store boundary, the system shall reject unknown kinds, empty identifiers, empty subjects, zero timestamps, and malformed metadata.

**AGG-07** When signals are scored, the system shall apply per-kind weights, invert coverage risk, clamp component values, and calculate a deterministic total.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/aggregator`
