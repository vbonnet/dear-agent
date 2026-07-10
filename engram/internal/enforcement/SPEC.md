# Engram Enforcement Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/enforcement` validates identity, plugin, and configuration
policy while emitting harness-neutral compliance telemetry.

## EARS Requirements

**EEF-01** When enforcement is disabled or its configuration is absent, the system shall allow validation without applying policy checks.

**EEF-02** When identity is required, the system shall reject a missing identity and an identity whose domain is not in a non-empty allowed-domain list.

**EEF-03** When identity is not required or no domain allowlist is configured, the system shall not reject an otherwise valid identity.

**EEF-04** When required plugins are configured before a plugin registry is available, the system shall defer plugin validation until the registry is attached.

**EEF-05** When a plugin registry is available, the system shall reject missing required plugins and installed versions below configured minimums.

**EEF-06** When comparing valid semantic versions, the system shall use semantic ordering and shall fall back to lexical ordering for non-semantic values.

**EEF-07** When a custom error template is valid, the system shall render the violation, remediation actions, and help URL; otherwise, it shall return a safe plain error.

**EEF-08** When telemetry validation succeeds or fails, the system shall publish an overall validation event without changing the validation result.

**EEF-09** When telemetry validation fails, the system shall classify and publish a harness-neutral violation event with available domain or plugin details.

**EEF-10** When no event bus is configured, the system shall perform standard enforcement validation without requiring telemetry.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_governance_runtime_guardrails.feature`
- Package tests: `engram/internal/enforcement/*_test.go`
