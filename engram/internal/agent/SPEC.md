# Engram Agent Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/agent` identifies the active coding-agent environment for
Engram telemetry and adaptation without changing the harness-neutral storage
contract.

## EARS Requirements

**EAD-01** When a supported agent environment variable is present, the system shall identify Claude Code, Cursor, Windsurf, or Aider before inspecting filesystem markers.

**EAD-02** When no supported agent environment variable is present, the system shall inspect supported agent-specific filesystem markers in deterministic priority order.

**EAD-03** When no supported environment or filesystem marker is present, the system shall return the unknown agent value.

**EAD-04** When detection succeeds or falls back to unknown, the system shall cache that result for subsequent calls on the same detector instance.

**EAD-05** When the detector cache is cleared, the system shall perform detection again on the next call.

**EAD-06** When multiple supported markers are present, the system shall apply the documented detection priority consistently.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/agent/*_test.go`
