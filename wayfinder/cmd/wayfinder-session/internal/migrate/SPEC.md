# Wayfinder Migration Engine Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Detailed explicit migration from retired files and status to canonical V2.

## EARS Requirements

**WAYFINDER-MIGRATE-01** When a valid retired status is converted, the system shall preserve project metadata, timestamps, terminal state, and requested session identity tags.

**WAYFINDER-MIGRATE-02** When retired phases converge on one canonical phase, the system shall merge phase history in canonical order without losing outcomes.

**WAYFINDER-MIGRATE-03** When migration runs in dry-run mode, the system shall produce a validation report without modifying status or artifact files.

**WAYFINDER-MIGRATE-04** When legacy artifacts are migrated, the system shall map them to canonical phase filenames and remove obsolete originals only after successful output creation.

**WAYFINDER-MIGRATE-05** When migration needs test outlines or BDD assets, the system shall derive substantive content from input and shall not check in boilerplate-only feature stubs.

**WAYFINDER-MIGRATE-06** When conversion encounters invalid schema, zero required timestamps, unsafe paths, or inconsistent data, the system shall reject migration with a reportable error.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/migrate/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
