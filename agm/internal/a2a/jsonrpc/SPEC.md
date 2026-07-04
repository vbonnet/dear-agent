# A2A JSON-RPC Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/jsonrpc` maps A2A protocol messages to JSON-RPC 2.0
`agent.message` payloads and back, including standard success and error
responses.

## EARS Requirements

**A2A-RPC-01** When an A2A protocol message is converted to JSON-RPC, the system shall emit JSON-RPC version `2.0`, method `agent.message`, message fields in params, and a stable message-number-derived ID.

**A2A-RPC-02** When a JSON-RPC message is converted to an A2A protocol message, the system shall validate the JSON-RPC envelope, status, required params, and resulting protocol message.

**A2A-RPC-03** When a JSON-RPC request uses an unsupported version or method, the system shall reject the request with a validation error.

**A2A-RPC-04** When a JSON-RPC request includes both result and error fields, the system shall reject the message as structurally invalid.

**A2A-RPC-05** When creating response messages, the system shall preserve the caller-supplied ID and use standard JSON-RPC success or error fields.

**A2A-RPC-06** When marshaling JSON-RPC messages, the system shall produce indented JSON and surface JSON parse errors on unmarshal.

## BDD Traceability

- Feature: `agm/test/bdd/features/mcp_parity.feature`
- Package tests: `agm/internal/a2a/jsonrpc/jsonrpc_test.go`
