# Agent-to-Agent Session Transport Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/a2a`.

## Overview

`pkg/a2a` exposes interactive agent sessions through the standard A2A task
protocol. The transport, task lifecycle, and default card contract support all
active harnesses without embedding a model-provider assumption.

## EARS Requirements

**A2A-01** When a session card declares a supported harness, the system shall advertise that harness in the default card name, skill description, and tags.

**A2A-02** When a session card omits its harness, the system shall advertise a harness-neutral agent session without a fabricated harness tag.

**A2A-03** When a caller supplies card presentation fields, the system shall preserve the explicit name, skills, provider, and capabilities.

**A2A-04** When an A2A server is created, the system shall require a card and session handler before binding a listener.

**A2A-05** When an A2A server binds, the system shall publish its resolved invocation URL, protocol version, and JSON-RPC transport through the well-known card.

**A2A-06** When a session asks for input, the system shall transition the task to input-required and resume the same task after the supervisor answers.

**A2A-07** When a session emits output and completes, the system shall make the terminal agent text available to the client.

**A2A-08** When a running task is cancelled, the system shall unblock any pending input request and terminate the task safely.

**A2A-09** When the in-memory bus publishes concurrently, the system shall isolate topics, bound slow-subscriber behavior, and drain subscribers on close.

**A2A-10** When shutdown occurs before or after server start, the system shall release the listener without waiting indefinitely.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/a2a`
