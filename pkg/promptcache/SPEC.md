# Prompt Cache Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**PROMPTCACHE-01** When Anthropic cache policy is requested, the system shall return explicit ephemeral cache control for the selected default or persistent tier.

**PROMPTCACHE-02** When OpenAI, Gemini, GLM, DeepSeek, Nemotron, or Qwen cache policy is requested, the system shall preserve provider-default behavior without emitting Anthropic cache-control fields.

**PROMPTCACHE-03** When an unknown model family is requested, the system shall return an error instead of silently choosing a provider policy.

**PROMPTCACHE-04** When a prompt snapshot is recorded, the system shall store a stable content hash, conservative token estimate, source, and timestamp.

**PROMPTCACHE-05** When observed cache reads are at or below five percent of the snapshot estimate, the system shall record a cache-break event.

**PROMPTCACHE-06** When changed content causes a cache break, the system shall write a private bounded diagnostic diff when possible.

**PROMPTCACHE-07** When post-compaction suppression is active, the system shall not report cache-break false positives.

**PROMPTCACHE-08** When the tracked-source limit is reached, the system shall evict the oldest snapshot before adding a new source.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/promptcache`
