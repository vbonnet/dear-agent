# Acceptance Criteria Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/acceptance`.

## Overview

`pkg/acceptance` loads and validates machine-readable Wayfinder acceptance
criteria. The same criteria contract governs work regardless of which harness
or model family implements it.

## EARS Requirements

**ACCEPT-01** When the acceptance file or acceptance section is absent, the system shall return an empty criterion set without failing the workflow.

**ACCEPT-02** When acceptance YAML is malformed, the system shall return a parsing diagnostic.

**ACCEPT-03** When criterion types are parsed, the system shall accept tests-pass, lint-clean, no-regressions, graceful-exit, handoff-confidence, and custom criteria.

**ACCEPT-04** When a runnable criterion omits its command, the system shall reject the criterion with its list index.

**ACCEPT-05** When a custom criterion omits both description and command, the system shall reject the empty criterion.

**ACCEPT-06** When declarative no-regressions, graceful-exit, or handoff-confidence criteria omit a command, the system shall retain them as valid policy checks.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/acceptance`
