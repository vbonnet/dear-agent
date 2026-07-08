# Dear Agent MCP Server Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/dear-agent-mcp` exposes the workflow engine and durable source store as a
JSON-RPC 2.0 MCP server. The server keeps stdio as the production transport,
offers an HTTP shim for tests and local debugging, and delegates durable state
to SQLite-backed workflow and source adapters instead of keeping transport-local
state.

## EARS Requirements

**DAM-01** When the server initializes, the system shall return MCP protocol version `2024-11-05`, a tools capability, server name `dear-agent-mcp`, and the current server version.

**DAM-02** When the client lists tools with a configured source adapter, the system shall expose workflow lifecycle tools and source tools in one MCP tool list.

**DAM-03** When the client lists tools without a configured source adapter, the system shall omit `FetchSource` and `AddSource`.

**DAM-04** When `workflow_run` is called with a valid workflow file, the system shall enqueue a pending run in the workflow database, return the queued run ID, and direct the client to poll `workflow_status`.

**DAM-05** When `workflow_run` is called without a file or with a missing file, the system shall return an MCP parameter error without creating a run.

**DAM-06** When `workflow_status` is called for an unknown run, the system shall return the domain not-found error code.

**DAM-07** When `workflow_approve` or `workflow_reject` is called, the system shall record the HITL decision through the workflow engine, default the approver to `mcp` when omitted, and preserve role and reason for audit.

**DAM-08** When a HITL approval is missing, already resolved, or assigned to a different role, the system shall map the workflow error to a stable MCP error code.

**DAM-09** When `workflow_cancel` is called, the system shall cancel the run through the workflow engine and default reason and actor to MCP-specific values when omitted.

**DAM-10** When `FetchSource` or `AddSource` is called with a backend expectation that does not match the configured adapter, the system shall fail fast with a backend mismatch error that names expected and actual backends.

**DAM-11** When `AddSource` is called without a URI, the system shall return an MCP parameter error without writing to the source store.

**DAM-12** When `FetchSource` receives `after` or `before` filters, the system shall accept only RFC3339 timestamps and shall pass valid bounds to the source adapter.

**DAM-13** When stdio receives malformed JSON, the system shall return a JSON-RPC parse error and continue serving subsequent requests.

**DAM-14** When HTTP mode serves a request, the system shall return exactly one JSON-RPC response envelope with `application/json` content type.

## BDD Traceability

- Feature: `agm/test/bdd/features/mcp_command_guardrails.feature`
- Package tests: `cmd/dear-agent-mcp/workflow_test.go`
- Package tests: `cmd/dear-agent-mcp/source_test.go`

