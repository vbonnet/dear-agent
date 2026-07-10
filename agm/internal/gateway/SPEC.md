# AGM Gateway Middleware Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/gateway` composes inspection, scope, rate-limit, circuit-breaker,
audit, dual-mode routing, and handoff policy around tool gateway calls.

## Requirements

**GTW-01** When a tool call enters the gateway, the system shall validate method shape before applying scope, rate-limit, circuit-breaker, and audit middleware.

**GTW-02** When scope policy denies a tool, the system shall reject the call and apply denylist precedence over allowlist entries.

**GTW-03** When a tool exceeds its configured rate or opens its circuit, the system shall reject that tool without blocking unrelated tools.

**GTW-04** When a circuit timeout elapses, the system shall permit a half-open probe and close or reopen the circuit from its result.

**GTW-05** When a gateway call completes, the system shall audit success or failure with method, tool, duration, and preserved handler outcome.

**GTW-06** When routing dual-mode work, the system shall select local execution or handoff from maintained complexity and mode rules.

**GTW-07** When generating a handoff, the system shall validate confidence, preserve artifacts and next steps, and support deterministic context serialization.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/internal/gateway/*_test.go`
