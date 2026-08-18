# SPEC: cmd/dear-agent-mcp/SPEC.md
# RELATED-SPEC: cmd/recommendation-mcp/SPEC.md
Feature: MCP command guardrails
  MCP command binaries should carry executable SPEC traceability so workflow,
  source, recommendation, and signal surfaces do not drift away from the
  shared JSON-RPC contract.

  Scenario Outline: MCP command packages declare SPEC coverage
    Given MCP command package "<package>" is configured
    When AGM validates MCP command package coverage
    Then MCP command package "<package>" should have a co-located SPEC

    Examples:
      | package                |
      | cmd/dear-agent-mcp     |
      | cmd/recommendation-mcp |
