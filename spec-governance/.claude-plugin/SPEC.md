# SPEC governance plugin manifest specification

<!-- Last audited at: 2026-07-31 -->

**Status:** Active
**Scope:** The Claude-compatible manifest that projects the canonical SPEC
governance skills into native plugin discovery.

## EARS Requirements

**DECL-SPEC-GOV-01** When a Claude-compatible harness loads the SPEC governance plugin, the system shall expose the canonical plugin identity and the `write-spec` and `audit-specs` skill directory.

**DECL-SPEC-GOV-02** If the SPEC governance plugin manifest is invalid, then the system shall reject plugin activation.

**DECL-SPEC-GOV-03** When the plugin manifest exposes a skill, the system shall treat the native manifest as a discovery projection and shall not make it an independent owner of that skill's workflow.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Feature: `agm/test/bdd/features/marketplace_parity.feature`
