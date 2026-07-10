# Engram Platform Runtime Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/platform` composes configuration, agent detection, telemetry,
events, plugins, and retrieval into the Engram runtime.

## EARS Requirements

**EPL-01** When platform initialization begins, the system shall load layered configuration before constructing dependent runtime components.

**EPL-02** When a supported agent harness is detected, the system shall expose that detected agent and update the runtime platform configuration without changing harness-neutral data contracts.

**EPL-03** When telemetry is enabled, the system shall register ecphory correctness, persona effectiveness, and Wayfinder return-on-investment listeners.

**EPL-04** When platform initialization is cancelled, the system shall stop initialization and close already-created resources.

**EPL-05** When plugin loading or event-handler registration fails, the system shall close the event bus and telemetry collector before returning the error.

**EPL-06** When the configured Engram path cannot initialize ecphory retrieval, the system shall continue with retrieval unavailable because ecphory is optional.

**EPL-07** When initialization succeeds, the system shall expose configuration, detected agent, telemetry, event bus, plugin registry, and optional ecphory components.

**EPL-08** When the platform closes, the system shall close initialized components in reverse dependency order and collect cleanup failures.

**EPL-09** When a component close operation panics, the system shall recover the panic and report it as a cleanup error rather than crashing the process.

**EPL-10** When close is called on a nil, partial, or previously closed platform, the system shall complete safely.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_governance_runtime_guardrails.feature`
- Package tests: `engram/internal/platform/*_test.go`
