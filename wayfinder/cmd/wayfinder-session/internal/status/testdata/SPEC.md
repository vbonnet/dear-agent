# Wayfinder Status Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-STATUS-01** When Wayfinder status tests load fixtures, the system shall preserve valid schema 2.0, minimal schema 2.0, and invalid-cycle states.

**FIX-STATUS-02** If invalid cyclic status is accepted or valid schema 2.0 state is rejected, the system shall fail the status scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
