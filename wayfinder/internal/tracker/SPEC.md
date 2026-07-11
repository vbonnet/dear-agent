# Wayfinder Core Tracker Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Core session lifecycle event persistence.

## EARS Requirements

**WAYFINDER-CORE-TRACKER-01** When a tracker is created, the system shall use an explicit event-path override when configured.

**WAYFINDER-CORE-TRACKER-02** When no event-path override exists, the system shall create the documented event destination with contained permissions.

**WAYFINDER-CORE-TRACKER-03** When the event destination cannot be initialized, the system shall return an error instead of dropping lifecycle events silently.

**WAYFINDER-CORE-TRACKER-04** When session and canonical phase lifecycle methods are called, the system shall append structured events in call order.

**WAYFINDER-CORE-TRACKER-05** When close is called, the system shall release the event destination without losing prior events.

## Test Traceability

- Package tests: `wayfinder/internal/tracker/tracker_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
