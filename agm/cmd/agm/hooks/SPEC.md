# AGM Command Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-CMD-HOOK-01** When a command hook validates a test session operation, the system shall distinguish isolated test sessions from production sessions.

**AGM-CMD-HOOK-02** If a production session is targeted by a test-only operation, the system shall reject the operation with remediation guidance.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
