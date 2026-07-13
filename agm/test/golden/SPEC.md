# AGM Configuration Golden Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-GOLDEN-01** When AGM configuration tests compare golden files, the system shall preserve canonical default, full, minimal, workspace, timeout, lock, and round-trip shapes.

**FIX-GOLDEN-02** If serialized configuration drifts from a golden contract, the system shall fail the comparison with the changed shape.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
