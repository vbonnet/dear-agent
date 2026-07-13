# AGM Dolt Migration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-DOLT-MIGRATION-01** When an AGM Dolt migration is applied, the system shall create the declared schema objects in migration order.

**AGM-DOLT-MIGRATION-02** If a migration cannot preserve required session data, the system shall fail before advancing the schema version.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
