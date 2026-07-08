# Engram Hook Runtime Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/hooks` is the runtime registry, security validator, executor, runner,
and report aggregator for Engram verification hooks. It provides the
harness-neutral execution layer used by hook-capable integrations while keeping
subprocess execution constrained by explicit command allowlists and optional
binary hash verification.

## EARS Requirements

**EHR-01** When a hook is registered, the system shall reject empty names, unsupported events, priorities outside 1 through 100, unsupported hook types, empty commands, negative timeouts, and timeouts above 600 seconds.

**EHR-02** When a duplicate hook name is registered, the system shall return `ErrDuplicateHook` and shall not replace the existing hook.

**EHR-03** When hooks are selected by event, the system shall return only matching hooks sorted by descending priority.

**EHR-04** When hooks are saved, the system shall create the hook directory with private permissions, write TOML to a temporary file, and atomically rename it into place.

**EHR-05** When hooks are loaded from a missing registry file, the system shall initialize an empty registry without error.

**EHR-06** When an allowed-command file is missing, the system shall keep the default command allowlist.

**EHR-07** When command validation receives a path-shaped command, the system shall require an exact allowlist entry for that path and shall not allow it by basename.

**EHR-08** When a hook command hash is configured, the system shall compare the normalized SHA-256 hash of the resolved command binary before execution.

**EHR-09** When a command is not allowed or its hash mismatches, the system shall return a failed verification result with a high-severity security violation.

**EHR-10** When a hook executes, the system shall invoke the configured command directly without a shell and shall bound execution by the hook timeout or the default timeout.

**EHR-11** When hook stdout is valid verification JSON, the system shall return that parsed result with the hook name and duration attached.

**EHR-12** When hook stdout is not valid verification JSON, the system shall derive pass or fail status from the process exit code and stderr.

**EHR-13** When hook execution times out or output reaches the maximum output size, the system shall return a warning verification result and a typed error.

**EHR-14** When multiple hooks run for an event, the system shall cap concurrent execution, continue after individual hook errors, record warnings, and calculate exit code 1 for failures, 2 for warnings-only, and 0 for all-pass.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks/*_test.go`

