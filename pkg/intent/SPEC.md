# Work Intent Board Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**INTENT-01** When work intent is declared, the system shall require a session identifier and at least one file or package scope.

**INTENT-02** When work intent is persisted, the system shall assign a unique identifier, deduplicate scope entries, and apply the default or explicit lifetime.

**INTENT-03** When intents are listed, the system shall support session filtering and exclude expired entries unless requested.

**INTENT-04** When an unknown intent is read or removed, the system shall return the typed not-found error.

**INTENT-05** When expiration runs, the system shall remove expired intent records and return the removal count.

**INTENT-06** When active intents from different sessions share a file or package, the system shall report the overlap.

**INTENT-07** When the same session shares scope with itself, the system shall not report a conflicting overlap.

**INTENT-08** While any supported harness and model family declares work, the system shall preserve identical lifetime, persistence, and overlap semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/intent`
