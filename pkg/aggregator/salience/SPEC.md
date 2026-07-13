# Signal Salience Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/aggregator/salience`.

## Overview

`pkg/aggregator/salience` classifies health signals into notification tiers,
drops noise, and applies a concurrency-safe sliding-window notification budget.

## EARS Requirements

**SALIENCE-01** When a salience tier is parsed or decoded, the system shall accept known names and numbers and reject values outside the supported ordering.

**SALIENCE-02** When a signal kind is validated, the system shall reject unknown kinds and invalid signal fields.

**SALIENCE-03** When an ingested signal has no explicit tier, the system shall classify it with the configured classifier or the default classifier.

**SALIENCE-04** When noise dropping is enabled, the system shall discard noise-tier signals before notification budgeting.

**SALIENCE-05** When a notification budget is exhausted, the system shall suppress budgeted tiers while allowing configured bypass tiers.

**SALIENCE-06** When the budget window advances, the system shall expire old notifications and make capacity available concurrently and deterministically.

**SALIENCE-07** When JSONL input contains a malformed signal, the system shall report the failing line instead of silently accepting partial corruption.

**SALIENCE-08** When outcomes are summarized, the system shall count accepted, dropped, suppressed, and invalid signals by tier.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/aggregator/salience`
