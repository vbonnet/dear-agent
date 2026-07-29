# Wayfinder Retrospective Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical rewind context and retrospective persistence.

## EARS Requirements

**WFR-01** When rewind magnitude is calculated, the system shall use the canonical CHARTER-through-RETRO sequence and reject unknown phase names.

**WFR-02** When a rewind has zero magnitude, the system shall skip retrospective and history writes.

**WFR-03** When a non-zero rewind is logged, the system shall capture reason, learnings, git state, canonical deliverables, and completed waypoint state.

**WFR-04** When rewind context is captured from status, the system shall parse canonical schema 2.0 status and preserve project identity as the session identifier.

**WFR-05** When a rewind entry is persisted, the system shall append it to `RETRO-retrospective.md` without rewriting existing entries.

**WFR-06** When a rewind entry is persisted, the system shall also append a structured event to `WAYFINDER-HISTORY.jsonl`.

**WFR-07** When git or context capture fails, the system shall preserve the rewind operation and record available diagnostic context.

**WFR-08** When concurrent context probes run, the system shall wait for every probe and shall preserve partial successful results.

**WFR-09** When a retrospective timestamp is recorded, the system shall normalize it to UTC for cross-host comparison.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/retrospective/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
