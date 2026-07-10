# Wayfinder Command Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical Wayfinder command entrypoint.

## EARS Requirements

**WAYFINDER-MAIN-01** When the Wayfinder binary starts, the system shall execute the canonical root command and map command errors to a nonzero exit.

**WAYFINDER-MAIN-02** When help is requested, the system shall expose the canonical session surface without registering retired direct executors.

**WAYFINDER-MAIN-03** When the binary is built, the system shall compile independently of a model provider or harness SDK.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder/main_smoke_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
