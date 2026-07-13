# Workspace Dolt Adapter Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**DOLT-01** When a component is registered, the system shall validate its storage prefix and reject reserved or colliding prefixes.

**DOLT-02** When component metadata is listed, read, updated, or removed, the system shall preserve dependency and status constraints.

**DOLT-03** When migration files are loaded, the system shall validate naming, version order, checksum, and declared dependencies.

**DOLT-04** When an unapplied migration succeeds, the system shall record component, version, checksum, actor, and created tables.

**DOLT-05** When an applied migration checksum differs, the system shall report drift rather than silently reapply it.

**DOLT-06** When a Dolt server starts, the system shall use the workspace-specific configuration and support bounded status, stop, commit, and log operations.

**DOLT-07** When workspace database initialization runs, the system shall create the required database and server configuration without selecting a harness.

**DOLT-08** While storage is used by any supported harness and model family, the system shall preserve identical schema, migration, and component isolation.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/workspace/dolt`
