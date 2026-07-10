# Wayfinder Legacy Converter Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Explicit conversion of retired status into canonical V2 state.

## EARS Requirements

**WAYFINDER-CONVERTER-01** When conversion receives nil or structurally invalid legacy status, the system shall reject the input.

**WAYFINDER-CONVERTER-02** When legacy status is converted, the system shall map retired phases into the canonical nine-phase sequence.

**WAYFINDER-CONVERTER-03** When multiple retired phases map to one canonical phase, the system shall merge status, timestamps, history, and outcomes deterministically.

**WAYFINDER-CONVERTER-04** When project metadata is missing, the system shall derive documented canonical defaults and initialize roadmap and quality metrics.

**WAYFINDER-CONVERTER-05** When blocked or terminal legacy state is converted, the system shall preserve the corresponding canonical status meaning.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/converter/converter_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
