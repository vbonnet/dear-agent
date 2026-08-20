# Research Pipeline Plugin Manifest Specification

<!-- Last audited at: 2026-07-26 -->

## EARS Requirements

**DECL-RESEARCH-PIPELINE-PLUGIN-01** When a Claude-compatible harness discovers the research-pipeline plugin, the system shall expose the canonical plugin identity and the root research-pipeline skill.

**DECL-RESEARCH-PIPELINE-PLUGIN-02** If the research-pipeline plugin manifest is invalid, the system shall reject plugin activation.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
