# AGM Container Reaper Test Script Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-DOCKER-TEST-01** When containerized reaper tests run, the system shall isolate fixtures and exercise success, missing-binary, and timeout behavior.

**AGM-DOCKER-TEST-02** If a container scenario violates its expected exit contract, the system shall fail the test suite.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
