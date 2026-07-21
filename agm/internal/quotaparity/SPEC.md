# Quota Monitoring Harness Parity Specification

<!-- Last audited at: 2026-07-21 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Context usage, cost, rate-limit, and model-family pricing policy for active AGM harnesses.

## Overview

Quota monitoring parity means AGM exposes a truthful quota record for every
active harness and supported model family. Claude Code has the richest native
statusline feed. Codex CLI, AGY, and OpenCode may lack equivalent native quota
APIs, and Pi exposes transcript usage without a provider quota API, so they must persist manifest-backed context/cost data and explicitly
report unavailable rate-limit or price data instead of silently substituting
Claude-specific defaults.

## EARS Requirements

**QMP-01** When AGM monitors an active harness, the system shall declare a context usage source for that harness.

**QMP-02** When AGM monitors an active harness, the system shall declare a cost source for that harness.

**QMP-03** When AGM monitors an active harness, the system shall declare a rate-limit source or an explicit unavailable policy for that harness.

**QMP-04** When AGM renders statusline data, the system shall persist quota data through manifest fields that are harness-neutral.

**QMP-05** When Claude Code statusline data is fresh, the system shall prefer exact statusline cost, context, model, and rate-limit values.

**QMP-06** When Claude Code statusline data is stale or absent, the system shall fall back to manifest or token-estimate data without treating stale rate limits as fresh.

**QMP-07** When a non-Claude active harness lacks native quota telemetry, the system shall show unavailable quota fields or manifest-derived estimates rather than Claude-specific defaults.

**QMP-08** When AGM reports cost for a model family, the system shall use the shared pricing table only when a model is known.

**QMP-09** When AGM reports cost for an unknown model family, the system shall mark the family explicitly unpriced rather than applying Opus pricing.

**QMP-10** When an active harness or supported model family is added, the system shall require quota parity tests for monitoring surfaces and pricing policy coverage.

**QMP-11** When GLM, DeepSeek, Nemotron, or Qwen is a supported default family, the system shall require sourced shared pricing rather than accepting an explicitly-unpriced placeholder.

**QMP-12** When AGM reports Pi quota data, the system shall identify Pi JSONL usage and manifest estimates as the context and cost sources while marking provider rate-limit telemetry unavailable.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_parity.feature`
