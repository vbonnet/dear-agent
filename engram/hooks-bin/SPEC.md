# Engram Hook Runtime Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**ENGRAM-HOOK-RUNTIME-01** When an Engram lifecycle hook runs, the system shall validate session context before reading or writing memory state.

**ENGRAM-HOOK-RUNTIME-02** If validation or health checks fail, the system shall return a failing status with diagnostic output.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
