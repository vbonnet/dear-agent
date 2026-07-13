# A2A Supervisor Client Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/a2a/client`.

## Overview

`pkg/a2a/client` discovers or directly connects to an A2A endpoint, sends one
task, handles protocol-native input requests, and returns terminal agent text.

## EARS Requirements

**A2ACLIENT-01** When a client is created from a card base URL, the system shall resolve the well-known agent card and use its advertised invocation endpoint.

**A2ACLIENT-02** When a client is created from an invocation endpoint, the system shall connect without requiring card discovery.

**A2ACLIENT-03** When a task enters input-required and an answer callback exists, the system shall send the callback response in the same task context.

**A2ACLIENT-04** When a task enters input-required without an answer callback, the system shall return an actionable error rather than inventing an answer.

**A2ACLIENT-05** When a task reaches a terminal state, the system shall return the final agent text and surface unsuccessful terminal states as errors.

**A2ACLIENT-06** When a zero-value or nil client is used, the system shall return a diagnostic or close safely without panicking.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/a2a/client`
