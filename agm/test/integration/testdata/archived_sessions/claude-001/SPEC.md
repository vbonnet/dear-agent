# Archived Claude Session Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-ARCHIVED-01** When integration tests load the archived Claude session fixture, the system shall preserve its archived manifest identity and lifecycle state.

**FIX-ARCHIVED-02** If archived-session handling mutates the fixture into an active state, the system shall fail the integration expectation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
