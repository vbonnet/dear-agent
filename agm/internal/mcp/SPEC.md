# AGM MCP Integration Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/mcp` owns AGM's session-facing MCP integration. It merges
environment, team, user, and session MCP configuration; detects available global
HTTP/SSE MCP servers; and gives AGM sessions a single lifecycle boundary for
connecting to global or session-local MCP servers.

## EARS Requirements

**MCP-INT-01** When AGM loads MCP configuration with a project path, the system shall merge environment, team, user, and session configuration with later scopes overriding earlier servers by name.

**MCP-INT-02** When session-specific MCP configuration is missing, the system shall treat the session configuration as empty instead of failing session startup.

**MCP-INT-03** When a named global MCP server is configured, the system shall health-check it before creating an HTTP/SSE MCP connection.

**MCP-INT-04** When a global MCP server is available, the system shall create a client session without terminating the shared server during AGM cleanup.

**MCP-INT-05** When a global MCP server is unavailable and a matching session MCP exists, the system shall fall back to the session-scoped MCP configuration.

**MCP-INT-06** When an MCP connection is disconnected more than once, the system shall treat the later disconnect as successful no-op cleanup.

**MCP-INT-07** When MCP configuration is supplied through `AGM_MCP_SERVERS`, the system shall require `name=url` pairs and reject malformed entries.

**MCP-INT-08** When MCP processes are cleaned up, the system shall only terminate session-owned processes and shall preserve global shared MCP processes.

## BDD Traceability

- Feature: `agm/test/bdd/features/mcp_parity.feature`
- Package tests: `agm/internal/mcp/config_hierarchy_test.go`
- Package tests: `agm/internal/mcp/manager_test.go`
- Package tests: `agm/internal/mcp/detector_test.go`
- Package tests: `agm/internal/mcp/process_cleanup_test.go`
