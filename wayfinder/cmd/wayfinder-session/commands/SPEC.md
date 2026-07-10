# Wayfinder Session Command Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Operator-facing Wayfinder V2 session commands.

## EARS Requirements

**WFC-SESSION-01** When a session is started, the system shall create and validate canonical V2 status with one of the nine canonical waypoints.

**WFC-SESSION-02** When `next-phase` reads status, the system shall parse only canonical V2 state and shall direct legacy state to explicit migration.

**WFC-SESSION-03** When a phase is started or completed, the system shall enforce canonical transition, git, history, validation, and tracker gates.

**WFC-SESSION-04** When a session is ended, the system shall update canonical V2 completion state and publish the session completion event.

**WFC-SESSION-05** When status is displayed, the system shall render canonical V2 project, waypoint history, and remaining phases.

**WFC-SESSION-06** When task commands receive a phase, the system shall require a canonical descriptive phase name.

**WFC-SESSION-07** When a rewind targets a phase, the system shall require a canonical prior phase and append the reason and context to the RETRO deliverable and history.

**WFC-SESSION-08** When lifecycle state is changed, the system shall validate the A2A-compatible state and its required diagnostic fields.

**WFC-SESSION-09** When coordination starts multiple projects, the system shall validate project directories and apply concurrency and sandbox policy.

**WFC-SESSION-10** When legacy status must be converted, the system shall expose V1 handling only through explicit `migrate` or `migrate-all` commands.

**WFC-SESSION-11** When a force or destructive option is requested, the system shall require the configured override guard and justification.

**WFC-SESSION-12** When start, start-phase, complete-phase, or set-lifecycle-state encounters legacy status, the system shall reject normal execution with explicit migration guidance.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/commands/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
