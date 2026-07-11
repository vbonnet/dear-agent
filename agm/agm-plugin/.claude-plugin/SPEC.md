# AGM Plugin Manifest Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-AGM-PLUGIN-01** When a Claude-compatible harness loads the AGM plugin, the system shall expose the canonical plugin identity and command integrity metadata.

**DECL-AGM-PLUGIN-02** If the plugin manifest or command hash is invalid, the system shall reject plugin activation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
