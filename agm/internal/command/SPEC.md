# AGM Command Translation Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/command` defines the harness-neutral command translation contract
for operations such as session rename, working-directory update, and lifecycle
hook execution. Implementations translate AGM commands to the control surface
available for a specific harness or model family.

## EARS Requirements

**CMDT-01** When a command translator is called, the system shall accept a context so timeout and cancellation semantics are shared across harness implementations.

**CMDT-02** When a harness does not support a requested command, the system shall return `ErrNotSupported` so callers can degrade gracefully.

**CMDT-03** When a harness API call fails, the system shall wrap the failure with `ErrAPIFailure` while preserving the underlying error.

**CMDT-04** When Gemini rename is requested, the system shall translate it to the Gemini conversation-title update operation.

**CMDT-05** When Gemini directory context is requested, the system shall translate it to a Gemini metadata update that records the working directory.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
