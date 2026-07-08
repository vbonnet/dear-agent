# LLM Config Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/llm/config` loads per-tool model configuration for Engram and workflow
callers. It supports tilde expansion, YAML parsing, provider aliases, tool-level
overrides, global defaults, and hardcoded fallbacks so callers can select models
without embedding provider-specific policy.

## EARS Requirements

**LLM-CONFIG-01** When a config path starts with `~`, the system shall expand it against the current user home directory before reading.

**LLM-CONFIG-02** When the config file does not exist, the system shall return a default config rather than an error.

**LLM-CONFIG-03** When a config file exists, the system shall parse it as YAML and shall return an error that names the path when reading or parsing fails.

**LLM-CONFIG-04** When defaults are missing, the system shall populate Anthropic and Gemini hardcoded defaults.

**LLM-CONFIG-05** When a default provider exists with an empty model, the system shall fill the provider's hardcoded default model.

**LLM-CONFIG-06** When a tool-specific provider model is configured, the system shall select it before global defaults.

**LLM-CONFIG-07** When no tool-specific provider model is configured, the system shall select the global provider default when available.

**LLM-CONFIG-08** When no config model is available, the system shall select hardcoded Anthropic and Gemini defaults and shall return an empty model for unknown providers.

**LLM-CONFIG-09** When max tokens are requested, the system shall return tool-specific provider limits before global provider limits and shall return zero when no positive limit is configured.

**LLM-CONFIG-10** When provider aliases are used, the system shall treat `claude` as Anthropic and `google` as Gemini.

## BDD Traceability

- Feature: `agm/test/bdd/features/llm_runtime_guardrails.feature`
- Package tests: `pkg/llm/config/loader_test.go`
- Package tests: `pkg/llm/config/example_test.go`

