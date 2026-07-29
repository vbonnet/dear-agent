# LLM Auth Specification

## Overview

`pkg/llm/auth` selects authentication methods and retrieves API keys for LLM
provider families. It keeps provider-specific credential policy explicit,
prefers managed Google Cloud authentication where supported, supports local
providers without secrets, and redacts keys before they can be logged.

## EARS Requirements

**LLM-AUTH-01** When Anthropic or Claude authentication is detected and `GOOGLE_CLOUD_PROJECT` is set, the system shall select Vertex AI authentication before checking API keys.

**LLM-AUTH-02** When Anthropic or Claude authentication has no Vertex AI project and `ANTHROPIC_API_KEY` is set, the system shall select API-key authentication.

**LLM-AUTH-03** When Gemini or Google authentication is detected and `GOOGLE_CLOUD_PROJECT` is set, the system shall select Vertex AI authentication before checking API keys.

**LLM-AUTH-04** When Gemini or Google authentication has no Vertex AI project and either `GEMINI_API_KEY` or `GOOGLE_API_KEY` is set, the system shall select API-key authentication.

**LLM-AUTH-05** When OpenRouter authentication is detected, the system shall select API-key authentication only when `OPENROUTER_API_KEY` is set.

**LLM-AUTH-06** When OpenAI authentication is detected, the system shall select API-key authentication only when `OPENAI_API_KEY` is set.

**LLM-AUTH-07** When Ollama or local authentication is detected, the system shall select local authentication without requiring any secret.

**LLM-AUTH-08** When an API key is requested, the system shall read only the provider-specific environment variables and shall return a provider-specific missing-key error when none are set.

**LLM-AUTH-09** When an API key is validated, the system shall enforce provider-specific key prefixes for Anthropic, Gemini, OpenRouter, and OpenAI.

**LLM-AUTH-10** When an API key is sanitized, the system shall fully mask short keys and shall preserve only the first eight and last four characters for longer keys.

**LLM-AUTH-11** When a Claude OAuth refresh succeeds remotely but rotated credentials cannot be persisted, the system shall quarantine the on-disk refresh token before returning, and every shared resolver entry point shall honor that quarantine.

**LLM-AUTH-12** While a credential-scoped refresh-stop marker exists, the system shall allow reads of an already-fresh access token but shall refuse every network refresh until the operator explicitly clears the stop.

**LLM-AUTH-13** When a spent or possibly spent refresh token cannot be quarantined, the system shall attempt to persist the credential-scoped refresh-stop marker before returning.

## BDD Traceability

- Feature: `agm/test/bdd/features/llm_runtime_guardrails.feature`
- Package tests: `pkg/llm/auth/hierarchy_test.go`
- Package tests: `pkg/llm/auth/apikey_test.go`
- Package tests: `pkg/llm/auth/oauth_refresh_test.go`
- Package tests: `pkg/llm/auth/oauth_refresh_extra_test.go`
- Package tests: `pkg/llm/auth/oauth_refresh_stop_test.go`
