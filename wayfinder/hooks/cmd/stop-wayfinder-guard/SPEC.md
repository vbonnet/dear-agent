# Wayfinder Stop Guard Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Harness-neutral stop checks for canonical Wayfinder sessions.

## EARS Requirements

**WAYFINDER-STOP-01** When a stop event has no Wayfinder project, the system shall complete without blocking the agent session.

**WAYFINDER-STOP-02** When canonical V2 status is parseable, the system shall classify completed and abandoned sessions as terminal and warn for active or blocked sessions.

**WAYFINDER-STOP-03** When legacy status is encountered, the system shall skip phase interpretation and require explicit migration rather than treating retired phases as canonical.

**WAYFINDER-STOP-04** When a session is complete or at RETRO, the system shall require a substantive `RETRO-retrospective.md` in a supported artifact location.

**WAYFINDER-STOP-05** When open beads are detected, the system shall warn with closure guidance without making the optional tracker a hard dependency.

**WAYFINDER-STOP-06** When deprecated short-name artifacts remain in the project root, the system shall warn that they are misplaced migration remnants.

**WAYFINDER-STOP-07** When any supported harness invokes the stop guard, the system shall apply the same canonical status and retrospective contract without selecting a model provider.

## Test Traceability

- Package tests: `wayfinder/hooks/cmd/stop-wayfinder-guard/main_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
