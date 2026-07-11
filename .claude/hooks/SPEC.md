# Claude Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**CLAUDE-HOOK-01** When Claude Code invokes a repository lifecycle guard, the system shall apply the same shared policy contract used by other active harnesses.

**CLAUDE-HOOK-02** If Claude Code attempts a bypassed pull-request or task operation, the system shall direct it to the corresponding safe wrapper.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
