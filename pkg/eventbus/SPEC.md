# Event Bus Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/eventbus`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**EVENTBUS-01** When an event is created, the system shall preserve event type, derived channel, timestamp, source, severity, and payload.

**EVENTBUS-02** When local subscribers match an event, the system shall invoke each subscriber with the published event.

**EVENTBUS-03** When a subscriber is removed, the system shall stop delivery to that subscription without affecting other subscribers.

**EVENTBUS-04** When publication occurs concurrently, the system shall protect subscriber registration and event delivery from data races.

**EVENTBUS-05** When a JSONL sink receives an event, the system shall append one valid JSON object per line.

**EVENTBUS-06** When a logging sink receives an event, the system shall emit the configured structured event fields.

**EVENTBUS-07** When context cancellation or sink errors occur, the system shall return cancellation to the publisher and shall isolate and log sink errors.

**EVENTBUS-08** While events originate from any supported harness and model family, the system shall preserve the same event schema, subscription, and sink semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/eventbus`
