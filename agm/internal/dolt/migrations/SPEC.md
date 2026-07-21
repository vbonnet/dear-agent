# AGM Dolt Migration Specification

<!-- Last audited at: 2026-07-20 -->

## EARS Requirements

**AGM-DOLT-MIGRATION-01** When an AGM Dolt migration is applied, the system shall create the declared schema objects in migration order.

**AGM-DOLT-MIGRATION-02** If a migration cannot preserve required session data, the system shall fail before advancing the schema version.

**AGM-DOLT-MIGRATION-03** When migration 018 is applied, the system shall add a nullable `tmux_session_revision` column without changing existing session rows, so MySQL/Dolt and isolated SQLite stores can use the same opaque tmux-identity revision and provisional resume-ownership semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/harness_parity.feature`
