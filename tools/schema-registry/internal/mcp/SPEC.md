# Schema Registry MCP Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Model Context Protocol transport for provider-neutral schema-registry operations.

## EARS Requirements

**SCHEMA-MCP-01** When the server is constructed, the system shall bind it to the configured workspace and verbosity mode.

**SCHEMA-MCP-02** When an initialize request is received, the system shall return valid MCP server capabilities and identity metadata.

**SCHEMA-MCP-03** When a tools-list request is received, the system shall expose the supported schema-registry tool definitions.

**SCHEMA-MCP-04** When a request omits a method or names an unknown method, the system shall return a JSON-RPC error.

**SCHEMA-MCP-05** When any supported harness or model family invokes the server, the system shall use the same MCP method and tool contracts.

## Test Traceability

- Package tests: `tools/schema-registry/internal/mcp/server_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
