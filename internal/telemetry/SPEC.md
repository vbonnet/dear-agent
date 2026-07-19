# Internal Telemetry Specification

<!-- Last audited at: 2026-07-03 -->

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

## Purpose

`internal/telemetry` provides OpenTelemetry instruments and tracing helpers for
agent, AGM session, DEAR, evaluation, stall, token, and human-typing
observability. Call sites should be able to record telemetry without knowing
whether a real provider or the default no-op provider is installed.

## EARS Requirements

**TEL-01** When telemetry instruments are first requested, the system shall create them from the global OpenTelemetry meter provider using the shared agent instrumentation scope.

**TEL-02** When a task starts or completes, the system shall record active and completed task metrics with low-cardinality provider, model, and status attributes.

**TEL-03** When token usage or stall duration is recorded, the system shall emit measurements with provider, model, and caller-supplied attributes.

**TEL-04** When session lifecycle telemetry is emitted, the system shall include session, provider, model, status, and role attributes when those values are available.

**TEL-05** When the human-typing guard detects, stashes, or over-captures input, the system shall record the corresponding metric without blocking prompt delivery.
