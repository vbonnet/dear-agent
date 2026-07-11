# Wayfinder Session Tracker Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Session and canonical phase lifecycle event publication.

## EARS Requirements

**WAYFINDER-SESSION-TRACKER-01** When a tracker is created, the system shall use the configured event path override or create the documented default event file.

**WAYFINDER-SESSION-TRACKER-02** When the event path cannot be created, the system shall return an initialization error.

**WAYFINDER-SESSION-TRACKER-03** When session or canonical phase lifecycle methods are called, the system shall append the corresponding structured event without selecting a harness or model provider.

**WAYFINDER-SESSION-TRACKER-04** When a tracker is closed, the system shall flush and close its event destination safely.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/tracker/tracker_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
