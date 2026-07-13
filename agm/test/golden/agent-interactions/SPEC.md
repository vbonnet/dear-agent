# AGM Agent Interaction Golden Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-INTERACTION-01** When agent interaction tests load golden responses, the system shall preserve success, provider error, missing-session, empty-message, and context-window cases.

**FIX-INTERACTION-02** If an adapter response drifts from the golden protocol shape, the system shall fail the interaction contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
