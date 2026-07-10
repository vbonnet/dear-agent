# Engram Harness Effort Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/harnesseffort` resolves canonical task-effort tiers into the
native configuration surfaces available to Codex, OpenCode, and Gemini-based
harnesses while retaining provider-neutral override support.

## EARS Requirements

**EHE-01** When harness effort configuration is loaded, the system shall start from embedded defaults and merge company overrides before user overrides.

**EHE-02** When an override file is absent, the system shall treat it as empty; when it is malformed or unreadable, the system shall return an error identifying that file.

**EHE-03** When configurations are merged, the system shall override only supplied fields and shall not mutate either input configuration.

**EHE-04** When model aliases are resolved, the system shall substitute known aliases and preserve unknown concrete model identifiers, including custom provider models.

**EHE-05** When Codex output is generated, the system shall create or replace only the Engram-managed TOML profile block and preserve unmanaged user content.

**EHE-06** When OpenCode output is generated, the system shall preserve unmanaged agents, generate deterministic tier-provider agents and commands, and let Engram-managed entries win conflicts.

**EHE-07** When a provider is not one of the built-in Anthropic, OpenAI, or Google providers, the system shall still generate an OpenCode agent using the configured model identifier unchanged.

**EHE-08** When Gemini guidance is generated, the system shall emit deterministic tier aliases using configured Google models with documented fallbacks.

**EHE-09** When generation is limited to a supported native harness surface, the system shall emit only that harness's file output or guidance.

**EHE-10** When no harness filter is supplied, the system shall generate every supported native harness-effort surface.

**EHE-11** When a caller requests dry-run generation, the system shall return proposed outputs without writing configuration files.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_governance_runtime_guardrails.feature`
- Package tests: `engram/internal/harnesseffort/*_test.go`
