# Recommendation MCP Server Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/recommendation-mcp` exposes the recommendation aggregator as a read-only
JSON-RPC 2.0 MCP server. It lets MCP clients inspect collected signals, ranked
recommendations and trend buckets without knowing the underlying SQLite schema.

## EARS Requirements

**RMS-01** When the server initializes, the system shall return MCP protocol version `2024-11-05`, a tools capability, server name `recommendation-mcp`, and the current server version.

**RMS-02** When the client lists tools, the system shall expose `get_signals`, `get_recommendations`, and `get_signal_trends`.

**RMS-03** When `get_signals` is called without a kind, the system shall query all known signal kinds from the store.

**RMS-04** When `get_signals` is called with kind, subject, since, or limit filters, the system shall validate the filters and pass bounded query parameters to the store.

**RMS-05** When `get_signals` is called with an unknown kind, invalid timestamp, or limit above the maximum, the system shall return an MCP parameter error.

**RMS-06** When `get_recommendations` is called, the system shall reduce signals to the most recent value per kind and subject, apply default or caller-provided weights, and return ranked recommendations.

**RMS-07** When `get_recommendations` is called with an unknown weight kind, invalid window, non-positive window, or `top_n` above the maximum, the system shall return an MCP parameter error.

**RMS-08** When `get_signal_trends` is called, the system shall require a valid signal kind and emit fixed-width buckets, including empty buckets, across the requested window.

**RMS-09** When `get_signal_trends` receives an invalid bucket, a bucket below the minimum, or a bucket count above the maximum, the system shall return an MCP parameter error.

**RMS-10** When an unknown tool such as the retired `suggest_backlog` surface is called, the system shall return an MCP method-not-found error.

**RMS-11** When any recommendation MCP tool succeeds, the system shall not mutate the signals database.

**RMS-12** When stdio receives malformed JSON, the system shall return a JSON-RPC parse error and continue serving subsequent requests.

**RMS-13** When HTTP mode serves a request, the system shall return exactly one JSON-RPC response envelope with `application/json` content type.

## BDD Traceability

- Feature: `agm/test/bdd/features/mcp_command_guardrails.feature`
- Package tests: `cmd/recommendation-mcp/main_test.go`
- Package tests: `cmd/recommendation-mcp/tools_test.go`
