# Wayfinder Session Migration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**WF-SESSION-MIGRATION-01** When Wayfinder session storage is upgraded, the system shall apply migrations in deterministic order without discarding canonical V2 state.

**WF-SESSION-MIGRATION-02** If a migration fails, the system shall not advance the recorded schema version.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
