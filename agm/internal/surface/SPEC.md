# AGM Operation Surface Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

`agm/internal/surface` defines AGM operations once and uses those definitions to
generate CLI and MCP surfaces. It is the shared control-plane contract for
session listing, lookup, search, status, archive, kill, and operation discovery.
Installed plugin Markdown is owned separately by the live Cobra tree.

## Requirements

**AGM-SURFACE-01** When operation surfaces are registered, the system shall include read operations, mutation operations, and meta operations in one registry.

**AGM-SURFACE-02** When list-sessions surfaces are generated, the system shall expose status, harness, limit, and offset request fields.

**AGM-SURFACE-03** When generated CLI commands validate enum inputs, the system shall reject values outside each operation's declared enum set.

**AGM-SURFACE-04** When generated MCP tools execute operations, the system shall create an operation context, defer cleanup, map operation errors to MCP errors, and return successful operation results.

**AGM-SURFACE-05** When operation discovery is exposed, the system shall publish `list_ops` as an MCP-only meta operation.

**AGM-SURFACE-06** When active harness filters are exposed, the system shall include Claude Code, Codex CLI, AGY, OpenCode, Pi, deprecated Gemini compatibility, and all-harness filtering.

**AGM-SURFACE-07** When operation definitions change, the system shall keep generated CLI and MCP code derived from the registry rather than hand-maintained divergent schemas.

**AGM-SURFACE-08** While the Cobra tree owns installed plugin command contracts, the operation registry shall not declare a second Skill surface for those commands.

**AGM-SURFACE-09** When the kill-session operation is generated for CLI or MCP, the request schema shall expose both the recent-activity `force` bypass and the active-harness `confirmed_stuck` confirmation used by the shared operation.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
- `agm/test/bdd/features/mcp_parity.feature` validates operation discovery parity.
