# Autoconfiguration Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/autoconfig`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**AUTOCONFIG-01** When a project name is hashed, the system shall return a stable shortened SHA-256 identifier.

**AUTOCONFIG-02** When no persisted baseline exists, the system shall return an empty baseline with the default rolling-window size.

**AUTOCONFIG-03** When a session is added to a full baseline, the system shall retain only the newest configured window and shall recompute averages and percentiles.

**AUTOCONFIG-04** When a baseline or generated configuration is persisted, the system shall create private directories and files.

**AUTOCONFIG-05** When retrospective proposals exceed magnitude or count bounds, the system shall omit the excess proposals from the applied configuration.

**AUTOCONFIG-06** When accepted proposals are applied, the system shall persist the generated configuration and append an auditable modification record.

**AUTOCONFIG-07** When post-change quality or cost fails for three consecutive monitored sessions, the system shall suspend the configuration and recommend reversion.

**AUTOCONFIG-08** When a reversion runs, the system shall remove the generated configuration and persist the reversion reason.

**AUTOCONFIG-09** While autoconfiguration is called from any supported harness and model family, the system shall apply the same bounds, persistence, and rollback semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/autoconfig`
