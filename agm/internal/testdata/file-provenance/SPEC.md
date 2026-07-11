# AGM File Provenance Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-PROVENANCE-01** When file-provenance tests load history and manifest fixtures, the system shall preserve declared file modifications and session ownership.

**FIX-PROVENANCE-02** If a fixture represents missing or inconsistent provenance, the system shall preserve the expected validation failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
