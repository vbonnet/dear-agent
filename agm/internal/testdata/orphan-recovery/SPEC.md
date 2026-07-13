# AGM Orphan Recovery Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-ORPHAN-01** When orphan-recovery tests load history and tracked manifests, the system shall preserve the declared process and session relationships.

**FIX-ORPHAN-02** If a fixture represents an orphan, the system shall preserve the expected recovery classification.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
