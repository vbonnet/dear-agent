# LLM Delegation Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/llm/delegation` chooses how an LLM request is executed when Engram tools
need agent output. It prefers available headless CLI execution for explicitly
requested providers, falls back to external API execution when allowed, and
preserves request/response metadata in a strategy-neutral shape.

## EARS Requirements

**LLM-DELEGATION-01** When a delegation strategy is requested with a provider override and the matching CLI is available, the system shall select the headless strategy.

**LLM-DELEGATION-02** When no headless strategy is available, the system shall fall back to the external API strategy when fallback is allowed.

**LLM-DELEGATION-03** When fallback is disabled and no headless strategy is available, the system shall return an error naming the provider.

**LLM-DELEGATION-04** When no provider override is supplied, the system shall default external API delegation to Anthropic.

**LLM-DELEGATION-05** When harness provider detection runs, the system shall map `CLAUDE_SESSION_ID` to Anthropic and `GEMINI_SESSION_ID` to Gemini.

**LLM-DELEGATION-06** When provider names are normalized, the system shall map `claude` to Anthropic and `google` to Gemini.

**LLM-DELEGATION-07** When headless availability is checked, the system shall require the `gemini`, `claude`, or `codex` binary for Gemini, Anthropic, or Codex respectively.

**LLM-DELEGATION-08** When available strategies are listed, the system shall include Headless only when available and shall always include ExternalAPI.

**LLM-DELEGATION-09** When a strategy error is created, the system shall preserve strategy name, operation, and wrapped error for `errors.Is` and `errors.As`.

## BDD Traceability

- Feature: `agm/test/bdd/features/llm_runtime_guardrails.feature`
- Package tests: `pkg/llm/delegation/strategy_test.go`
- Package tests: `pkg/llm/delegation/delegation_extra_test.go`

