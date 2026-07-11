# Wayfinder Batch Migration Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Backup, restore, dry-run, and workspace-scale explicit migration orchestration.

## EARS Requirements

**WAYFINDER-MIGRATION-01** When a project has no status file, the system shall reject single-project migration without creating output.

**WAYFINDER-MIGRATION-02** When a project already uses canonical V2 status, the system shall skip conversion without rewriting it.

**WAYFINDER-MIGRATION-03** When a retired project is migrated, the system shall create a recoverable backup before replacing status.

**WAYFINDER-MIGRATION-04** When restoration is requested, the system shall restore the selected backup to the project status path.

**WAYFINDER-MIGRATION-05** When dry-run is requested, the system shall validate and describe changes without modifying files.

**WAYFINDER-MIGRATION-06** When workspace migration runs, the system shall discover retired projects, skip canonical projects, honor concurrency, and aggregate success, skip, and failure results.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/migration/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
