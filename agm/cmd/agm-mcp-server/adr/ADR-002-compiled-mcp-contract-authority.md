# ADR-002: Compiled MCP contract authority

Status: Accepted

Date: 2026-08-01

## Context

AGM retained build-excluded generated CLI, MCP, and parity reference files.
They did not participate in the production server and described obsolete MCP
names. At the same time, `ops.ListOps` advertised `get_status` and
`list_workspaces`, which were not compiled MCP tools. A hash-only generator
check could not establish provider-visible behavior.

## Decision

The provider-visible MCP contract is owned by `registerMCPTools` and the typed
handlers in `agm/cmd/agm-mcp-server`. `surface.Registry` remains logical input
to the compiled-contract comparator, with finite compatibility records for
intentional differences. `ops.ListOps` owns the `agm_list_ops` discovery
catalog and must exactly project the compiled logical names.

Retire the AGM generator command, its build-excluded reference outputs, and the
hash-only verification target. Do not activate generated handlers, add aliases,
or change the ten compiled public tool names or schemas in this retirement.

## Consequences

- Discovery no longer advertises uncallable `get_status` or `list_workspaces`.
- Clients must use MCP `tools/list` for callable-tool discovery; `agm_list_ops`
  now reports only that same logical set.
- New provider-visible tools require registration, typed schema, exact
  discovery projection, comparator records where needed, architecture inventory,
  and tests in one change.
- If a future product requires `agm_get_status` or another generated name, it
  needs an explicit public-contract migration rather than reactivating retired
  output.

## Evidence

- `agm/cmd/agm-mcp-server/main.go`
- `agm/cmd/agm-mcp-server/tools.go`
- `agm/cmd/agm-mcp-server/surface_contract_test.go`
- `agm/cmd/agm-mcp-server/surface_contract_compatibility_test.go`
- `agm/internal/ops/introspect.go`
