# Gateway Package Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/gateway` is the transport-neutral dispatch layer for workflow control
commands. It normalizes command types, handler registration, structured errors,
event fanout, and workflow handler payloads so CLI, HTTP, and future adapters
share one in-process command contract.

## Requirements

**GATEWAY-CORE-01** When a gateway dispatches a command with a registered handler, the system shall call the handler with the original context and command.

**GATEWAY-CORE-02** When a gateway dispatches a command without a registered handler, the system shall return `CodeUnknownCommand` and echo the command ID.

**GATEWAY-CORE-03** When an adapter subscribes to events, the system shall allocate a buffered event channel and return an idempotent unsubscribe function.

**GATEWAY-CORE-04** When an adapter unsubscribes, the system shall remove the subscriber and close its event channel.

**GATEWAY-CORE-05** When events are published, the system shall deliver them to all current subscribers without blocking on full subscriber buffers.

**GATEWAY-CORE-06** When workflow run handling receives no runner, the system shall return `CodeUnavailable`.

**GATEWAY-CORE-07** When workflow run handling receives missing file args or non-string input values, the system shall return `CodeInvalidArgs`.

**GATEWAY-CORE-08** When workflow status handling cannot find a run, the system shall return `CodeNotFound`.

**GATEWAY-CORE-09** When workflow list or logs handling receives missing or invalid limits, the system shall apply the documented defaults and ceilings.

**GATEWAY-CORE-10** When HITL approve or reject handling receives no caller login, the system shall return `CodeUnauthorized`.

**GATEWAY-CORE-11** When HITL decision handling reports not found, already resolved, or role mismatch, the system shall map those cases to `CodeNotFound`, `CodeConflict`, and `CodeUnauthorized` respectively.

**GATEWAY-CORE-12** When cancel handling is invoked before workflow cancel support exists, the system shall return `CodeUnavailable`.

**GATEWAY-CORE-13** When workflow list handling receives an unknown non-empty run-state filter, the system shall return `CodeInvalidArgs` without querying workflow storage.

## BDD Traceability

- `agm/test/bdd/features/api_gateway_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
