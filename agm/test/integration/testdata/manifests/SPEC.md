# AGM Integration Manifest Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-INTEGRATION-MANIFEST-01** When integration tests load manifest fixtures, the system shall distinguish valid V2, missing-session, and invalid-schema records.

**FIX-INTEGRATION-MANIFEST-02** If an invalid manifest is accepted, the system shall fail the validation scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
