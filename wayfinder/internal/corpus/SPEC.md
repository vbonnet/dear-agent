# Wayfinder Corpus Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Workspace-isolated schema publication and cross-component discovery.

## EARS Requirements

**WAYFINDER-CORPUS-01** When Wayfinder schemas are requested, the system shall return project, canonical phase, and validation schemas with workspace isolation fields.

**WAYFINDER-CORPUS-02** When schema registration or publication is requested, the system shall invoke the configured Corpus Callosum binary rather than assuming a global executable name.

**WAYFINDER-CORPUS-03** When Corpus Callosum is unavailable, the system shall degrade gracefully for registration, publication, status, and query operations.

**WAYFINDER-CORPUS-04** When project, phase, AGM session, or Engram bead data is queried, the system shall preserve the requested workspace boundary.

**WAYFINDER-CORPUS-05** When cross-component discovery succeeds, the system shall return structured component data without embedding a model-provider dependency.

## Test Traceability

- Package tests: `wayfinder/internal/corpus/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
