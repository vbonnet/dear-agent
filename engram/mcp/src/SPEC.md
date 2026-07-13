# Engram MCP TypeScript Service Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**ENGRAM-MCP-TS-01** When an MCP request reaches the Engram TypeScript service, the system shall route it to the declared tool and return a protocol-valid response.

**ENGRAM-MCP-TS-02** When cached data is reused, the system shall honor cache freshness and shall not return an expired value as current.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
