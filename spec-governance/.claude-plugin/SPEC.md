# SPEC governance plugin manifest specification

<!-- Last audited at: 2026-07-31 -->

**Status:** Active
**Scope:** The isolated Claude-compatible manifest that projects the canonical
SPEC governance skills into native plugin discovery.

## EARS Requirements

**DECL-SPEC-GOV-01** When a Claude-compatible harness loads the SPEC governance plugin, the system shall expose exactly the canonical `write-spec` and `audit-specs` skill directories.

**DECL-SPEC-GOV-02** If the SPEC governance plugin manifest is invalid or declares an additional native surface, then the system shall reject plugin activation.

**DECL-SPEC-GOV-03** When the plugin manifest exposes a skill, the system shall treat the native manifest as a discovery projection and shall not make it an independent owner of that skill's workflow.

**DECL-SPEC-GOV-04** When the isolated plugin root is packaged, the system shall not expose plugin-level agents, hooks, MCP servers, or language servers.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Feature: `agm/test/bdd/features/marketplace_parity.feature`
