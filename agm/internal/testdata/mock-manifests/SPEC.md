# AGM Mock Manifest Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-MANIFEST-01** When manifest tests load mock sessions, the system shall preserve valid, archived, stale, duplicate, missing-tmux, and corrupted states.

**FIX-MANIFEST-02** If a manifest fixture is intentionally invalid, the system shall preserve the expected parser or validation failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
