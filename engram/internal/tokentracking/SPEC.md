# Engram Session Token Tracking Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/tokentracking` normalizes provider response usage, aggregates
session totals, assigns quota severity, and exports summaries.

## EARS Requirements

**ETT-01** When Anthropic usage metadata is received, the system shall extract input, output, cache-creation, and cache-read token counts.

**ETT-02** When OpenAI or OpenRouter usage metadata is received, the system shall map prompt, completion, and cached prompt tokens into the neutral usage record while preserving explicit OpenAI, GLM, DeepSeek, Nemotron, or Qwen family attribution.

**ETT-03** When Gemini usage metadata is received, the system shall map prompt, candidate, and cached-content token counts into the neutral usage record.

**ETT-04** When usage counts are negative, unsupported, or exceed the sanity limit, the system shall reject the usage record.

**ETT-05** When a neutral LLM response or legacy Claude response event is observed, the system shall accumulate token usage safely under concurrent delivery.

**ETT-06** When total response usage crosses the warning or error thresholds, the system shall retain the highest session severity observed.

**ETT-07** When no telemetry collector is available, the system shall record normalized usage directly through the session listener.

**ETT-08** When context-aware recording runs, the system shall use a neutral LLM span name and attach generation-usage attributes and the detected provider and model.

**ETT-09** When summaries are requested, the system shall return input, output, cache, total, response-count, duration, and severity fields in text or JSON form.

**ETT-10** When the default tracker is requested or reset, the system shall manage one standalone tracker instance without losing thread safety.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_observability_guardrails.feature`
- Package tests: `engram/internal/tokentracking/*_test.go`
