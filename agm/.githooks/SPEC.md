# AGM Git Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-GITHOOK-01** When an AGM commit or merge hook runs, the system shall execute the configured enforcement gates before accepting the Git operation.

**AGM-GITHOOK-02** If an enforcement gate fails, the system shall preserve the failing status and identify the rejected gate.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
