# Agent Trace Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/agenttrace`.

## Overview

`pkg/agenttrace` records tool calls, reasoning, state transitions, and memory
operations through OpenTelemetry. Its schema is provider neutral and treats
privacy as a prerequisite for observability.

## EARS Requirements

**TRACE-01** When a tool call is instrumented, the system shall record the tool identity, call identity, redacted arguments, redacted output, retry count, duration, and error state.

**TRACE-02** When reasoning is instrumented, the system shall record non-empty plan, action, observation, and next-decision attributes under stable GenAI keys.

**TRACE-03** When a state transition is instrumented, the system shall record source state, destination state, redacted context edits, and redacted handoff payload.

**TRACE-04** When a memory operation is instrumented, the system shall record operation, store, redacted query, relevance, freshness, and result count.

**TRACE-05** When structured trace content contains sensitive JSON keys, the system shall recursively replace their values before exporting the attribute.

**TRACE-06** When free-text trace content or an error contains a credential assignment, bearer credential, or recognized standalone provider key, the system shall replace the credential before recording attributes, status, or events.

**TRACE-07** When a free-text attribute exceeds the byte limit, the system shall truncate it on a valid UTF-8 boundary and append a visible truncation marker.

**TRACE-08** When tracing is disabled, the system shall allow every instrumentation helper to execute through the no-op provider without panicking.

**TRACE-09** When instrumented work panics, the system shall close the span as an error and re-raise the panic.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/agenttrace`
