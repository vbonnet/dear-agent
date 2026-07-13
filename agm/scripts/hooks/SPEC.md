# AGM Coordination Script Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-SCRIPT-HOOK-01** When an AGM coordination hook runs, the system shall derive session context from declared inputs before emitting coordination state.

**AGM-SCRIPT-HOOK-02** If the hook is invoked outside a supported session, the system shall exit without mutating unrelated coordination state.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
