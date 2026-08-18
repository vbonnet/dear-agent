# Backlog Specification

<!-- Last audited at: 2026-07-18 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/backlog`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**BACKLOG-01** When backlog markdown is parsed, the system shall preserve item identity, title, phase, priority, effort, status, dependencies, section, and file scope.

**BACKLOG-02** When status text includes annotations or formatting, the system shall classify pending, in-flight, blocked, and done states tolerantly.

**BACKLOG-03** When a dependency uses a phase wildcard, the system shall preserve and resolve that wildcard against phase completion.

**BACKLOG-04** When suggestions are computed, the system shall exclude non-pending items and items with unsatisfied dependencies.

**BACKLOG-05** When eligible items are ranked, the system shall apply explicit priority, phase ordering, and effort consistently with deterministic tie breaking.

**BACKLOG-06** When no item is eligible, the system shall return an empty suggestion set instead of fabricating work.

**BACKLOG-07** When suggestions are returned, the package shall not dispatch workers or mutate live work state; Beads and the caller own those operations.

**BACKLOG-08** While backlog operations are called from any supported harness and model family, the system shall preserve identical parsing, ranking, and dependency semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/backlog`
