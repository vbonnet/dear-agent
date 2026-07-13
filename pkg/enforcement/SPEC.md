# Enforcement Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/enforcement`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**ENFORCEMENT-01** When a pattern database is loaded, the system shall parse its YAML and shall reject an empty pattern collection.

**ENFORCEMENT-02** When detector patterns contain unsupported expressions, the system shall skip and report those pattern identifiers without disabling valid patterns.

**ENFORCEMENT-03** When RE2-only enforcement is requested, the system shall compile only the explicitly declared RE2-compatible expressions.

**ENFORCEMENT-04** When a command is evaluated for rejection, the system shall return the first active matching pattern in declared order.

**ENFORCEMENT-05** When pattern conditions depend on worktree context, the system shall evaluate those conditions before reporting a violation.

**ENFORCEMENT-06** When a violation is rendered, the system shall include its configured rationale and alternative approach rather than only a denial.

**ENFORCEMENT-07** When a violation is filed, the system shall preserve structured context in a deterministic private Markdown report.

**ENFORCEMENT-08** While enforcement is called from any supported harness and model family, the system shall apply the same pattern, context, ordering, and guidance semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/enforcement`
