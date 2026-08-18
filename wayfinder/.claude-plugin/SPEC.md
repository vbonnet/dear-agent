# Wayfinder Plugin Manifest Specification

<!-- Last audited at: 2026-07-17 -->

## EARS Requirements

**DECL-WF-PLUGIN-01** When a Claude-compatible harness discovers Wayfinder, the system shall expose the canonical plugin identity and root Wayfinder skill.

**DECL-WF-PLUGIN-02** If the Wayfinder plugin manifest is invalid, the system shall reject plugin activation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
