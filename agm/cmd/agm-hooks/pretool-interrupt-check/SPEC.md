# PreTool Interrupt Check Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`pretool-interrupt-check` enforces AGM interrupt flags before tool calls.

## EARS Requirements

**PIC-01** When no AGM session name can be determined, the system shall allow the tool call.

**PIC-02** When no interrupt flag exists for the session, the system shall allow the tool call.

**PIC-03** When a stop interrupt exists, the system shall consume the flag and block the tool call.

**PIC-04** When a kill interrupt exists, the system shall block the tool call without consuming the flag.

**PIC-05** When a steer interrupt exists, the system shall consume the flag, print guidance, and allow the tool call.

**PIC-06** When interrupt reading fails or the flag type is unknown, the system shall fail open.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/pretool-interrupt-check/main_test.go`

