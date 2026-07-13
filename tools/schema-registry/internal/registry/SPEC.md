# Schema Registry Persistence Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Workspace-scoped schema registration and SQLite persistence.

## EARS Requirements

**SCHEMA-REGISTRY-01** When a registry is opened, the system shall initialize workspace-scoped persistent storage and required tables.

**SCHEMA-REGISTRY-02** When a valid component schema is registered, the system shall persist its component, version, compatibility mode, and schema body.

**SCHEMA-REGISTRY-03** When the latest schema is requested, the system shall return the newest registered version for the component.

**SCHEMA-REGISTRY-04** When components are listed, the system shall return the persisted component metadata without duplicates.

**SCHEMA-REGISTRY-05** When a schema is unregistered, the system shall remove the targeted persisted record without deleting unrelated components.

**SCHEMA-REGISTRY-06** When workspace detection runs outside a recognized workspace path, the system shall use the documented fallback workspace.

## Test Traceability

- Package tests: `tools/schema-registry/internal/registry/db_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
