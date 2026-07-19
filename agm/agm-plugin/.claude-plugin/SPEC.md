# AGM Plugin Manifest Specification

<!-- Last audited at: 2026-07-17 -->

## EARS Requirements

**DECL-AGM-PLUGIN-01** When a Claude-compatible harness loads the AGM plugin, the system shall expose the canonical plugin identity and registered command and skill directories.

**DECL-AGM-PLUGIN-02** When registered plugin Markdown changes, the system shall verify per-file content hashes and the complete command and skill inventories rather than rely on an unconsumed aggregate hash.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
