# Engram MCP Server Specification

<!-- Last audited at: 2026-07-20 -->

## Scope

This specification governs the TypeScript MCP server in `engram/mcp/src`. The
server provides read-only access to Engram retrieval, plugin discovery, and
canonical Wayfinder status. It does not mutate Beads or project state.

## Requirements

**ENGRAM-MCP-01** When a client lists tools, the server shall publish exactly `engram.retrieve`, `engram.plugins.list`, and `wayfinder.phase.status` with the schemas defined in `ENGRAM-MCP-SERVER-API.md`.

**ENGRAM-MCP-02** When `engram.retrieve` receives a non-empty query, the server shall invoke the configured Engram CLI without a shell and pass optional tag and limit values as separate arguments.

**ENGRAM-MCP-03** When a retrieval limit is supplied, the server shall reject values that are not positive integers at most 1000.

**ENGRAM-MCP-04** When `engram.plugins.list` is called, the server shall inspect the core and user plugin directories beneath `ENGRAM_ROOT` and list only directories containing `plugin.yaml`.

**ENGRAM-MCP-05** When `wayfinder.phase.status` receives a project path, the server shall resolve it, read `WAYFINDER-STATUS.md`, and validate the complete document against canonical Wayfinder schema 2.0.

**ENGRAM-MCP-06** When Wayfinder status is valid, the server shall return the resolved `project`, canonical `phase`, derived `progress`, and canonical `status` without inferring state from legacy Markdown.

**ENGRAM-MCP-07** When Wayfinder status is invalid, the server shall return a diagnostic without partially parsed state.

**ENGRAM-MCP-08** The server shall expire cached results after the configured TTL and bound the cache to 200 entries.

**ENGRAM-MCP-09** When the platform delivers a change event for a watched plugin directory or Wayfinder status file, the server shall invalidate matching cached results.

**ENGRAM-MCP-10** When the server writes MCP protocol data, the server shall use stdout and reserve stderr for startup and fatal diagnostics.

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`
- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`

## Configuration

- `ENGRAM_CLI`: executable for retrieval; default `engram`.
- `ENGRAM_ROOT`: plugin root; default `~/.engram`.
- `MCP_CACHE_TTL_MS`: result-cache TTL in milliseconds; default `30000`.

## Verification

- `npm test` exercises cache and Wayfinder parsing behavior.
- `npm run build` type-checks and compiles the service.
- BDD guardrails ensure the active TypeScript Wayfinder consumer uses the
  canonical parser and omits retired Wayfinder vocabulary.

The focused source specification is [src/SPEC.md](./src/SPEC.md).
