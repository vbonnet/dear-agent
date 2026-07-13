# Health Checker Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/health-checker`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**HEALTH-01** When health results are classified, the system shall treat OK and informational states as healthy and warning and error states as issues.

**HEALTH-02** When sequential checks run, the system shall preserve declaration order and stop with partial results on context cancellation.

**HEALTH-03** When parallel checks run, the system shall preserve result indexes and convert panics into error results.

**HEALTH-04** When results are summarized, the system shall count passed, warning, error, and fixable outcomes consistently.

**HEALTH-05** When summary status is converted to an exit code, the system shall prioritize errors over warnings and warnings over success.

**HEALTH-06** When dry-run fixing is enabled, the system shall report fixable results without invoking fix functions.

**HEALTH-07** When a fix succeeds, the system shall mark the result healthy and remove stale fix metadata.

**HEALTH-08** When a fix fails, the system shall retain issue status and shall append failure context to the result.

**HEALTH-09** While health operations are called from any supported harness and model family, the system shall preserve identical execution, summary, and fixing semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/health-checker`
