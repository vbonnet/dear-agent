# Wayfinder status reader requirements specification

<!-- Last audited at: 2026-07-20 -->

**Status:** Active
**Scope:** Validated, read-only canonical status summaries for external consumers.

## EARS requirements

**WFREAD-01** When a consumer requests status from a project directory, the system shall parse and validate the complete canonical schema 2.0 status.

**WFREAD-02** When canonical status is missing, unreadable, or invalid, the system shall return an error without a partial summary or compatibility fallback.

**WFREAD-03** When canonical status is valid, the system shall expose only the project name, lifecycle status, current waypoint, update timestamp, completed-or-skipped waypoint progress, and bead references required by read-only consumers.

**WFREAD-04** When a consumer already holds status bytes, the system shall fully validate those exact bytes without requiring a second filesystem read.

## Traceability

- Tests: `wayfinder/cmd/wayfinder-session/statusread/statusread_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
