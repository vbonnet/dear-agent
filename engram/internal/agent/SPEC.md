# Engram Agent Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/agent` identifies the active coding-agent environment for
Engram telemetry and adaptation without changing the harness-neutral storage
contract.

## Detection Priority

Detection reads current inputs lazily on the first `Detect` call and after
`ClearCache`. Supported environment signals always precede filesystem signals:

1. Claude Code: `CLAUDECODE=1`, then non-empty `CLAUDE_CODE_ENTRYPOINT`.
2. Cursor: `CURSOR=1`, then non-empty `CURSOR_SESSION_ID`.
3. Windsurf: `WINDSURF=1`.
4. Aider: non-empty `AIDER_MODEL`, then non-empty `AIDER_ARCHITECT`.
5. Claude Code filesystem marker: `$HOME/.claude`.
6. Cursor working-directory marker: `.cursorrules`.
7. Windsurf working-directory marker: `.windsurfrules`.
8. Aider working-directory markers: `.aider.conf.yml`, then `.aiderignore`.
9. `AgentUnknown` when no supported signal exists.

If the home directory cannot be resolved, detection returns `AgentUnknown`
without inspecting working-directory markers. A cached result, including
`AgentUnknown`, remains stable until `ClearCache`.

## Concurrency and Cache Generations

`Detector` is safe for concurrent method calls and its zero value is ready for
use. Methods on one instance are serialized: concurrent cache misses share one
detection result, and `ClearCache` waits for an in-flight detection before it
invalidates that completed cache generation. A `Detect` call started after
`ClearCache` returns reads a new generation of current inputs. A `Detector`
must not be copied after first use.

## EARS Requirements

**EAD-01** When a supported agent environment variable is present, the system shall identify it before inspecting filesystem markers and shall apply the documented environment priority.

**EAD-02** When no supported agent environment variable is present, the system shall inspect supported agent-specific filesystem markers in the documented priority order.

**EAD-03** When no supported environment or filesystem marker is present, the system shall return the unknown agent value.

**EAD-04** When detection succeeds or falls back to unknown, the system shall cache that result for subsequent calls on the same detector instance.

**EAD-05** When the detector cache is cleared, the system shall perform detection again on the next call.

**EAD-06** When multiple supported environment or filesystem markers are present, the system shall apply the Detection Priority consistently.

**EAD-07** When detection and cache clearing occur concurrently on one detector instance, the system shall serialize those operations so each cache generation is evaluated at most once and an input evaluation that began before a cache clear cannot repopulate the cache after that clear completes.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/agent/*_test.go`
