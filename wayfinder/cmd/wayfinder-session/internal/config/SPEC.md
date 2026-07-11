# Wayfinder Configuration Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Workspace discovery and session storage configuration.

## EARS Requirements

**WAYFINDER-CONFIG-01** When default configuration is requested, the system shall return a valid canonical storage mode and workspace configuration.

**WAYFINDER-CONFIG-02** When configuration is validated, the system shall reject unsupported storage modes and invalid required paths.

**WAYFINDER-CONFIG-03** When dotfile storage is selected, the system shall resolve storage beneath the project directory.

**WAYFINDER-CONFIG-04** When centralized storage is selected, the system shall expand and use the configured central path.

**WAYFINDER-CONFIG-05** When workspace detection receives an explicit path or environment override, the system shall prefer that source before upward discovery.

**WAYFINDER-CONFIG-06** When configuration is saved and loaded, the system shall preserve validated values without broadening file permissions.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/config/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
