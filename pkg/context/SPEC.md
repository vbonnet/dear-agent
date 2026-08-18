# Context Management Parity Specification

<!-- Last audited at: 2026-07-21 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/context`.

## Overview

`pkg/context` detects context usage, resolves model-specific windows and zones,
and compacts conversations. Exact native telemetry is preferred, while every
supported harness has an explicit and visibly estimated fallback.

## EARS Requirements

**CONTEXT-01** When harness auto-detection runs, the system shall recognize Claude Code, Codex CLI, Pi CLI, Antigravity, OpenCode, and deprecated Gemini compatibility session identifiers.

**CONTEXT-02** When exact portable context counters are supplied as structured JSON, the system shall deterministically read nested used tokens, total tokens, and model identity without depending on a harness-specific transcript layout.

**CONTEXT-03** When exact portable context counters are supplied as text, the system shall parse comma-separated token usage values across supported harnesses.

**CONTEXT-04** When explicit context counters are malformed, negative, zero-total, over capacity, or outside the platform integer range, the system shall return an error instead of silently estimating replacement values.

**CONTEXT-05** When a supported harness does not expose exact counters, the system shall return a model-aware heuristic and shall mark the usage as estimated.

**CONTEXT-06** When a model override is supplied, the system shall preserve that model identifier and use its registered context window for percentage calculations.

**CONTEXT-07** When a supported model family default is resolved, the system shall provide a positive context window for Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen families.

**CONTEXT-08** When model-specific benchmark data is unavailable, the system shall use documented conservative thresholds with low confidence rather than fabricate high-confidence limits.

**CONTEXT-09** When usage crosses registered warning, danger, or critical thresholds, the system shall classify the corresponding context zone deterministically.

**CONTEXT-10** When conversation usage remains below the configured compaction threshold, the system shall preserve the original messages without calling the summarization provider.

**CONTEXT-11** When conversation compaction runs without an explicit model, the system shall delegate model selection to the configured provider instead of forcing a model family.

**CONTEXT-12** When conversation compaction succeeds, the system shall preserve the configured recent messages and prepend a tool-free compact summary.

**CONTEXT-13** When the summarization provider fails, the system shall return the original conversation and an error without discarding context.

**CONTEXT-14** When compaction safety cooldown or count limits apply, the system shall block repeated compaction according to the shared anti-loop policy.

**CONTEXT-15** When Pi supplies portable exact counters or requires a fallback, the system shall use Pi-specific session, usage, model, and message-count inputs and shall identify the result as `pi-cli`.

## BDD Traceability

- Feature: `agm/test/bdd/features/context_management_parity.feature`

## Test Traceability

- Unit package: `pkg/context`
