# Wayfinder Retrospective Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical rewind context and retrospective persistence.

## EARS Requirements

**WFR-01** When rewind magnitude is calculated, the system shall use the canonical CHARTER-through-RETRO sequence and reject unknown phase names.

**WFR-02** When an accepted rewind has zero magnitude, the system shall append the same canonical history event and retrospective block as any other accepted rewind while preserving magnitude zero.

**WFR-03** When a rewind is logged, the system shall capture reason, learnings, git state, canonical deliverables, and completed waypoint state.

**WFR-04** When rewind context is captured from status, the system shall parse the post-reset canonical schema 2.0 status and preserve project identity as the session identifier.

**WFR-05** When a rewind entry is persisted, the system shall append it to `RETRO-retrospective.md` without rewriting existing entries.

**WFR-06** When a rewind entry is persisted, the system shall also append a structured event to `WAYFINDER-HISTORY.jsonl`.

**WFR-07** When required status, history, or retrospective persistence fails, the system shall return an explicit error without a normal rewind-success claim; context probes may preserve available diagnostic context.

**WFR-08** When concurrent context probes run, the system shall wait for every probe and shall preserve partial successful results.

**WFR-09** When a retrospective timestamp is recorded, the system shall normalize it to UTC for cross-host comparison.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/retrospective/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
