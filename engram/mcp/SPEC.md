# Engram MCP server specification

<!-- Last audited at: 2026-07-17 -->

## Executable EARS requirements

**EMCP-01** When an MCP client lists tools, the system shall return `engram.retrieve`, `engram.plugins.list`, and `wayfinder.phase.status`.

**EMCP-02** When `engram.retrieve` receives untrusted values, the system shall pass them as child-process arguments without a shell.

**EMCP-03** When a retrieval limit is not a positive integer at most 1000, the system shall return an MCP error result.

**EMCP-04** When a cached entry has expired, the system shall discard it rather than return it as current.

**EMCP-05** When a watched plugin or Wayfinder path changes, the system shall invalidate matching cached entries.

**EMCP-06** While stdio is the active transport, the system shall write diagnostics only to stderr.

**EMCP-07** The system shall not require Python, sentence-transformers, or an embedding model to start the TypeScript MCP server.

## BDD traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`
- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`

## Executable owners

- `engram/mcp/src/index.ts`
- `engram/mcp/src/cache.ts`
- `engram/mcp/package.json`
