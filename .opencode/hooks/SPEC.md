# OpenCode Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**OPENCODE-HOOK-01** When OpenCode invokes a native before-tool repository guard, the system shall apply the shared routing, task, pull-request, and bypass policies.

**OPENCODE-HOOK-02** If OpenCode attempts a disallowed raw operation, the system shall return actionable guidance for the approved wrapper.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/hook_parity.feature`
