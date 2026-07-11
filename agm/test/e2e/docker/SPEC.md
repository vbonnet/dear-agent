# AGM End-to-End Container Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-E2E-DOCKER-01** When AGM container end-to-end tests run, the system shall build and compose the declared isolated test services.

**DECL-E2E-DOCKER-02** If a required test service cannot become ready, the system shall fail the end-to-end run.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
