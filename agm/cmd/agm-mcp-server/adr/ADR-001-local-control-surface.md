# ADR-001: Verified local AGM control surface

Status: Accepted

Date: 2026-07-17

## Context

AGM needs a structured local surface for discovery and lifecycle operations.
Earlier ADRs assumed filesystem manifests, a five-second cache, three read-only
tools, and metadata-only behavior. The current system uses shared operations over
Dolt and includes guarded mutations.

## Decision

Run MCP over stdio and delegate AGM behavior to `internal/ops`. Resolve and
verify an explicit Dolt workspace before registering tools. Open a storage
adapter per request rather than maintaining an MCP-specific session cache.

Expose the finite tool set registered in `main.go`: AGM reads, lifecycle
mutations, schema discovery, and two local Wayfinder status readers. Do not read
conversation history. Write diagnostics to stderr so stdout remains owned by the
protocol transport.

An optional A2A endpoint may publish read-only agent cards, but it is a separate
protocol surface and does not change the MCP transport decision. It binds the
configured `mcp_server.a2a.bind` address verbatim; non-loopback configuration is
network exposure and must be treated as such.

## Consequences

- CLI and MCP lifecycle behavior converge in shared operations.
- Startup fails loudly instead of advertising a handless tool surface.
- Storage reads are current at the operation boundary; there is no MCP cache to
  invalidate.
- Mutating tools inherit shared validation, provenance, dry-run, and rollback
  behavior.
- New tools must update registration, schemas, architecture inventory, tests,
  and privacy analysis together.

## Evidence

- `agm/cmd/agm-mcp-server/main.go`
- `agm/cmd/agm-mcp-server/tools.go`
- `agm/cmd/agm-mcp-server/workspace_guard_test.go`
- `agm/internal/ops`
