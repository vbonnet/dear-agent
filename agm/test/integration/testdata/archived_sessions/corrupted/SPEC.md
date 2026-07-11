# Corrupted Archived Session Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-CORRUPT-01** When integration tests load the corrupted archived-session fixture, the system shall preserve the malformed state used to exercise diagnostics.

**FIX-CORRUPT-02** If corrupted state is accepted as a valid archived session, the system shall fail the integration expectation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
