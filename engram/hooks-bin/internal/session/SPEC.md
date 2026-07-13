# Hook Session Identifier Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/session` extracts canonical session identifiers from
history JSONL and supplies collision-resistant fallbacks for degraded inputs.

## EARS Requirements

**EHSI-01** When history contains multiple entries, the system shall inspect the final line for the active session identifier.

**EHSI-02** When the final entry contains a lowercase canonical UUID, the system shall return it unchanged.

**EHSI-03** When history is missing, unreadable, empty, malformed, or contains an invalid UUID, the system shall return both a contextual error and an `auto-<timestamp>-<hex>` fallback identifier.

**EHSI-04** When random fallback generation is available, the system shall include random entropy in fallback identifiers.

**EHSI-05** When random fallback generation fails, the system shall still return a format-compatible timestamp-derived identifier.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/session/*_test.go`
