# AGM Database Migration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-MIGRATION-01** When AGM initializes or upgrades its database, the system shall apply schema migrations in deterministic version order.

**AGM-MIGRATION-02** If a migration statement fails, the system shall not record the failed migration as applied.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
