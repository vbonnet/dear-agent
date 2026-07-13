# Code Intelligence Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/codeintel`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**CODEINTEL-01** When a workspace is inspected, the system shall detect supported languages from their manifest files.

**CODEINTEL-02** When changed files are supplied, the system shall scope dead-code, dangling-reference, and debug-pattern checks to relevant languages.

**CODEINTEL-03** When a required build tool is unavailable, the system shall report a non-failing skipped result instead of executing an unrelated fallback.

**CODEINTEL-04** When a language build command fails, the system shall return an error-severity result with bounded diagnostic output.

**CODEINTEL-05** When Tier 1 AST rules are available, the system shall use embedded or explicitly configured rules and shall parse compact findings.

**CODEINTEL-06** When Tier 1 tooling is unavailable, the system shall preserve the Tier 0 analysis path.

**CODEINTEL-07** When source trees are scanned, the system shall exclude dependency directories and shall tolerate unreadable unrelated files.

**CODEINTEL-08** While code intelligence is called from any supported harness and model family, the system shall apply the same language registry, scopes, and severities.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/codeintel`
