# Wayfinder ESLint Context Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-ESLINT-01** When Wayfinder lint-context tests inspect ESLint configuration, the system shall preserve the declared legacy configuration shape.

**FIX-ESLINT-02** If ESLint context detection returns the wrong configuration family, the system shall fail the fixture scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
