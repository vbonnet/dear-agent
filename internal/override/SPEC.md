# Safety Override Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/override`.

## Overview

`internal/override` requires justified, audited bypasses of curated safety
gates. High-risk requests can use an independent language-model classifier,
but the policy API and audit vocabulary remain provider and harness neutral.

## EARS Requirements

**OVERRIDE-01** When an override omits a concrete justification, the system shall refuse the bypass with corrective guidance.

**OVERRIDE-02** When an override is evaluated, the system shall record the caller, gate, risk, reason, judge identity, and verdict in the audit sink.

**OVERRIDE-03** When a P0 or P1 override has no explicit judge, the system shall select an independent model-backed judge when a provider is configured.

**OVERRIDE-04** When a lower-risk override has no explicit judge, the system shall apply the deterministic offline judge.

**OVERRIDE-05** When the deterministic floor denies a reason, the system shall deny the override without consulting or being weakened by a model classifier.

**OVERRIDE-06** When an injected model classifier renders a verdict, the system shall identify the policy judge as `llm-override` without exposing the provider family in the contract.

**OVERRIDE-07** When the optional model classifier is unavailable after the deterministic floor allows a reason, the system shall retain the deterministic verdict.

**OVERRIDE-08** When a required judge cannot render any verdict, the system shall fail closed and audit the judge error.

**OVERRIDE-09** When a safety-critical caller requires a durable override audit, the system shall not authorize the bypass unless the complete verdict record and owner-only audit metadata have been synced to durable storage; an audit create, append, permission, sync, close, or path-identity failure shall deny the bypass.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/override`
