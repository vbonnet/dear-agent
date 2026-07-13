# Graceful Exit Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/gracefulexit`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**GRACEFUL-EXIT-01** While no repository override is present, the system shall enable the no-overfit guardrail by default.

**GRACEFUL-EXIT-02** When a repository configuration is missing, the system shall return the enabled zero-value configuration without error.

**GRACEFUL-EXIT-03** When the guardrail is disabled, the system shall require a non-empty rationale.

**GRACEFUL-EXIT-04** When enabled configuration is rendered, the system shall include the canonical guardrail and applicable task categories in the banner.

**GRACEFUL-EXIT-05** When disabled configuration is rendered, the system shall return an empty banner.

**GRACEFUL-EXIT-06** When task categories are returned, the system shall return a defensive copy of the canonical catalog.

**GRACEFUL-EXIT-07** While workers use any supported harness and model family, the system shall publish the same no-overfit rule and opt-out validation.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/gracefulexit`
