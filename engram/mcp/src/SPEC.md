# Engram MCP TypeScript Service Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**ENGRAM-MCP-TS-01** When an MCP request reaches the Engram TypeScript service, the system shall route it to the declared tool and return a protocol-valid response.

**ENGRAM-MCP-TS-02** When cached data is reused, the system shall honor cache freshness and shall not return an expired value as current.

**ENGRAM-MCP-TS-03** When the Wayfinder status tool reads a project, the system shall validate canonical schema-2.0 YAML with ordered history and completed non-skipped predecessors before deriving phase, progress, and status.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
