# Engram MCP Server (Go) - Specification

<!-- Last audited at: NEEDS-AUDIT -->

## Overview

`engram-mcp` is the Go MCP server for Engram tools. It supersedes the legacy
Python server (`ai-tools engram/mcp/engram_mcp_server.py`, archived repo)
whose `beads_create` tool appended to a private JSONL file
(`~/.beads/issues.jsonl` by default) and reported success while the bd/dolt
beads database never received the write. That silent write loss evaporated
four P0 disk-retro action items on 2026-07-03 (bead ce-ctsi).

## Objectives

1. **No silent write loss**: a `beads_create` success means the row is
   readable from the configured store; every failure is a hard error.
2. **No wrong-database writes**: the store is always addressed explicitly;
   bd auto-discovery and fallback stores are never used.
3. **Recovery**: orphaned writes stranded in a legacy JSONL store can be
   backfilled idempotently.
4. **Drop-in**: preserves the legacy server's read tools
   (`engram_retrieve`, `engram_plugins_list`, `wayfinder_phase_status`).
5. **Truthful identity**: the version advertised to MCP clients and logged at
   startup is the version stamped into the compiled server.

## EARS Requirements

**EMC-01** When beads_create is invoked while no beads database path is configured, the system shall return a hard error to the MCP caller and shall not write to any fallback store.

**EMC-02** When the bd create invocation fails or its output contains no bead ID, the system shall return a hard error to the MCP caller that includes the underlying bd error output.

**EMC-03** When bd acknowledges a create, the system shall verify by read-after-write (bd show against the same explicit --db path) that the row exists before reporting success.

**EMC-04** When the read-after-write verification cannot find the acknowledged bead, the system shall return a hard error identifying the bead ID and the store path.

**EMC-05** When the beads store is addressed, the system shall pass the database path explicitly on every bd invocation rather than relying on bd working-directory auto-discovery.

**EMC-06** When beads_reconcile runs against a legacy JSONL store, the system shall create only open legacy beads that are absent from the beads database and shall label each backfilled bead with its legacy source ID.

**EMC-07** When beads_reconcile encounters a legacy bead whose backfill-src label or title already exists in the beads database, the system shall skip that bead, making reconciliation idempotent.

**EMC-08** When beads_reconcile is invoked with dry_run enabled, the system shall report the beads that would be backfilled without performing any write.

**EMC-09** When wayfinder_phase_status reads a status file, the server shall return phase state only after complete canonical schema 2.0 validation.

**EMC-10** The `engram-mcp` executable shall report one consistent build-stamped version value across its observable interfaces.

**EMC-11** When `engram-mcp` starts, the system shall log its build-stamped version value.

**EMC-12** When an MCP client successfully initializes `engram-mcp`, the system shall return the same build-stamped version value as `serverInfo.version`.

## Tools

| Tool | Kind | Behaviour |
|------|------|-----------|
| `beads_create` | write | Verified create through `engram/internal/beadstore` (EMC-01..05) |
| `beads_reconcile` | write | Idempotent backfill from a legacy JSONL store (EMC-06..08) |
| `engram_retrieve` | read | Wraps `engram retrieve --format json` |
| `engram_plugins_list` | read | Scans `$ENGRAM_ROOT/{core,user}/plugins/*/plugin.yaml` |
| `wayfinder_phase_status` | read | Fully validates canonical schema 2.0, then reports `current_waypoint` and `status`; progress is unknown because the schema has no percentage field |

## Configuration

| Env var | Default | Purpose |
|---------|---------|---------|
| `BEADS_DB` | *(unset — beads tools hard-error)* | bd database path, e.g. `~/beads/context-engine/.beads` |
| `BD_PATH` | `bd` (PATH) | bd binary override |
| `ENGRAM_ROOT` | `~/.engram` | Plugin discovery root |
| `ENGRAM_CLI` | `engram` (PATH) | engram binary override |

`BEADS_DB` has deliberately no default: defaulting to a store nothing reads
is the root cause this server exists to fix.

## Testing Strategy

- `engram/internal/beadstore`: unit tests with an injected command runner,
  including a regression test reproducing the acknowledged-but-not-landed
  write (`TestVerifiedCreate_AcknowledgedButNotLanded`).
- `engram/cmd/engram-mcp`: unit tests for the read-tool ports and parsers.
- `engram/cmd/engram-mcp`: an artifact-level regression builds the real binary
  with a non-default version and verifies it through a raw MCP 2025-11-25
  initialize response over stdio.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
- Related SPEC: `engram/internal/beadstore/SPEC.md`
