# Wayfinder Quality Telemetry Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Append-only quality events and deterministic analytics export.

## EARS Requirements

**WAYFINDER-TELEMETRY-01** When a quality event is emitted, the system shall create missing parent directories and append one complete event record.

**WAYFINDER-TELEMETRY-02** When an event already has a timestamp, the system shall preserve it rather than replacing it.

**WAYFINDER-TELEMETRY-03** When an active span context exists, the system shall include trace and span identifiers in the event.

**WAYFINDER-TELEMETRY-04** When no active span context exists, the system shall omit trace identifiers without failing emission.

**WAYFINDER-TELEMETRY-05** When telemetry is read, the system shall return quality events and skip unrelated event records.

**WAYFINDER-TELEMETRY-06** When CSV is generated, the system shall emit stable headers and correctly quote special characters.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/telemetry/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
