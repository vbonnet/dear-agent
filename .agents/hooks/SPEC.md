# Agents Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGENTS-HOOK-01** When an Agents-compatible harness invokes a lifecycle guard, the system shall enforce the shared repository policy before allowing the action.

**AGENTS-HOOK-02** If a guarded action is denied, the system shall identify the approved replacement workflow and its rationale.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
