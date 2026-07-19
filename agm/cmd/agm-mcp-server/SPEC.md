# AGM MCP server specification

<!-- Last audited at: 2026-07-17 -->

## Executable EARS requirements

**MCS-01** When no workspace is supplied by environment or configuration, the system shall exit non-zero before registering MCP tools.

**MCS-02** When the selected workspace database is unreachable, the system shall exit non-zero before registering MCP tools.

**MCS-03** When `agm_create_session` receives a valid request, the system shall delegate to `ops.CreateSessionWithContext` with MCP caller provenance.

**MCS-04** When `agm_send_message` receives a valid request, the system shall delegate delivery to the shared operation with a tmux-capable context.

**MCS-05** When an AGM operation returns an `OpError`, the system shall return its RFC 7807 representation in an MCP error result.

**MCS-06** When a Wayfinder detail request contains path traversal characters, the system shall reject the session identifier.

**MCS-07** While stdio is the active MCP transport, the system shall write diagnostics only to stderr.

**MCS-08** The system shall not expose conversation history through the registered MCP tool surface.

## BDD traceability

- Feature: `agm/test/bdd/features/mcp_parity.feature`

## Executable owners

- `agm/cmd/agm-mcp-server/main.go`
- `agm/cmd/agm-mcp-server/tools.go`
- `agm/cmd/agm-mcp-server/wayfinder.go`
- `agm/internal/ops`
