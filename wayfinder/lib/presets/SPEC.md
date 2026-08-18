# Wayfinder Preset Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Safe loading of canonical Wayfinder workflow presets.

## EARS Requirements

**WAYFINDER-PRESET-01** When a core preset is requested, the system shall load and validate its test, specification, phase-gate, retrospective, and economic settings.

**WAYFINDER-PRESET-02** When different core presets are loaded, the system shall preserve their documented behavioral differences.

**WAYFINDER-PRESET-03** When an unknown or reserved preset name is requested, the system shall reject it.

**WAYFINDER-PRESET-04** When preset content exceeds the configured size limit, the system shall reject it before decoding.

**WAYFINDER-PRESET-05** When a custom preset directory is configured, the system shall contain lookup to that directory and reject path traversal.

**WAYFINDER-PRESET-06** When phase-gate settings are decoded, the system shall accept only canonical descriptive field names and reject unknown or retired phase-numbered keys.

## Test Traceability

- Package tests: `wayfinder/lib/presets/loader_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
