# Schema Validation Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: JSON Schema validation and compatibility checks.

## EARS Requirements

**SCHEMA-VALIDATE-01** When a schema document is registered, the system shall reject invalid JSON Schema structure.

**SCHEMA-VALIDATE-02** When data is validated against a schema, the system shall return validation errors for nonconforming data.

**SCHEMA-VALIDATE-03** When compatibility is checked, the system shall evaluate the requested backward, forward, full, or none mode.

**SCHEMA-VALIDATE-04** When incompatible schema changes are detected, the system shall report the compatibility violations.

**SCHEMA-VALIDATE-05** When a component name is validated, the system shall accept only documented component-name syntax.

**SCHEMA-VALIDATE-06** When a version is validated, the system shall require semantic-version syntax.

## Test Traceability

- Package tests: `tools/schema-registry/internal/schema/validator_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
