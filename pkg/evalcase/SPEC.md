# Evaluation Case Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/evalcase`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**EVALCASE-01** When a trace is classified, the system shall derive ordered tool, reasoning, state, memory, stall, score, and terminal-outcome failure classes from explicit evidence.

**EVALCASE-02** When evaluation candidates are extracted, the system shall preserve reproducible inputs, expected outcomes, provenance, and classification metadata.

**EVALCASE-03** When a trace has no configured problem signal, the system shall decline extraction instead of inventing an evaluation case.

**EVALCASE-04** When duplicate cases are encountered, the system shall use stable identity to avoid duplicate persistence.

**EVALCASE-05** When a case is stored, the system shall persist a deterministic representation and shall retain source traceability.

**EVALCASE-06** When read-only telemetry spans are bridged, the system shall group them by trace identifier and preserve relevant span attributes.

**EVALCASE-07** When the pipeline runs, the system shall report classification, extraction, and storage outcomes independently.

**EVALCASE-08** While evaluation processing is called from any supported harness and model family, the system shall preserve identical evidence thresholds and stored case semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/evalcase`
