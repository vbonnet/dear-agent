# Wayfinder Beads Adapter Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Optional canonical task-tracker integration.

## EARS Requirements

**WAYFINDER-BEADS-01** When the Beads database environment override is present, the system shall use that explicit database path.

**WAYFINDER-BEADS-02** When no database override is present, the system shall use the canonical context-engine database path.

**WAYFINDER-BEADS-03** When Beads availability is checked, the system shall report whether the configured executable can be resolved.

**WAYFINDER-BEADS-04** When a bead is created, the system shall pass the database, title, and machine-readable output arguments without shell interpolation.

**WAYFINDER-BEADS-05** When bead creation receives an empty title, the system shall reject the request before executing the tracker.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/beads/beads_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
