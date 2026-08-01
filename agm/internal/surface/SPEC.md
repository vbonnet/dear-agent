# AGM Operation Surface Specification

<!-- Last audited at: 2026-08-01 -->

## Overview

`agm/internal/surface` owns logical operation intent for session listing,
lookup, search, status, archive, kill, and operation discovery. It is a
comparator input, not an executable MCP generator. The provider-visible MCP
server is hand-registered and owns its established wire contract; a
compiled-surface audit reconciles that contract with this registry through
finite compatibility records. Installed plugin Markdown remains owned
separately by the live Cobra tree.

## Contract ownership

| Contract | Owner | Enforcement |
|---|---|---|
| Provider-visible MCP names and schemas | `agm/cmd/agm-mcp-server/registerMCPTools` and its typed handlers | SDK discovery and compatibility tests |
| Logical comparator input | `surface.Registry` | exact finite compatibility records |
| `agm_list_ops` discovery catalog | `ops.ListOps` | exact projection test against compiled logical names |

## Compiled MCP compatibility

| Registry operation | Provider-visible contract |
|---|---|
| `list_sessions` | `agm_list_sessions`; required nested `filters` with `status`, `agent_type`, and `limit`; root `fields`; no `offset` |
| `get_session` | compatibility name `agm_get_session_metadata`; required `identifier` |
| `get_session_output` | `agm_get_session_output`; required `identifier`; optional `lines`; exact match |
| `search_sessions` | `agm_search_sessions`; root `query`; required nested `filters` with `status` and `limit` |
| `get_status` | intentionally absent |
| `archive_session` | `agm_archive_session`; required `identifier`; live `dry_run` extension |
| `kill_session` | `agm_kill_session`; registry fields plus live `dry_run` extension |
| `list_ops` | `agm_list_ops`; empty input; output from the compiled-logical-name projection in `ops.ListOps` |

The finite schema exceptions are:

- `list_sessions` moves `status`, `harness`, and `limit` to
  `filters.status`, `filters.agent_type`, and `filters.limit`. It omits the
  registry's optional integer `offset` (description `Pagination offset`). The
  live wire adds a required, closed `filters` object and optional `fields` with
  type `null | array`, string items, and description `Field mask: only return
  these fields (e.g. [id, name, status]). Omit for all fields.` The live status
  and harness fields omit the registry enums. Their descriptions change from
  `Filter by session status` to `Filter by status: active (default), archived,
  or all`, and from `Filter by harness; gemini-cli is deprecated` to `Filter by
  harness: claude-code, codex-cli, agy, opencode-cli, pi-cli, gemini-cli, or
  all`. The limit description adds `, default 100`.
- `search_sessions` moves `status` and `limit` to `filters.status` and
  `filters.limit`, adding a required, closed `filters` object. The live status
  field omits the registry enum. Its description uses the same live status
  wording above; the limit description changes from `Maximum results to return
  (1-50)` to `Maximum results (1-50, default 10)`.
- `get_session` changes only the tool name from `agm_get_session` to
  `agm_get_session_metadata`; its `identifier` schema remains exact.
- `get_status` is the one exact registry operation omitted from the compiled
  server.
- `archive_session` adds optional boolean `dry_run` with description `Preview
  the archive without executing. Returns what would happen.` Its identifier
  description changes from `Session ID, name, or UUID prefix` to `Session ID,
  name, or tmux session name to archive`.
- `kill_session` adds optional boolean `dry_run` with description `Preview the
  kill without executing. Returns what would happen.` Its identifier
  description ends in `tmux session name to kill`; the live `force` description
  adds a final period; and the live `confirmed_stuck` description adds
  `Required for an active session.`

All unlisted schema dimensions and every registry tool description match
exactly. Each item above is represented by separate operation-, path-, and
dimension-specific records in the compiled contract test; no prose row acts as
a wildcard.

The compiled server also owns four exact non-registry tools:
`agm_create_session`, `agm_send_message`,
`engram_list_wayfinder_sessions`, and `engram_get_wayfinder_session`.
`ops.ListOps` projects these names together with the compiled logical registry
operations and does not advertise uncompiled operations.

## Requirements

**AGM-SURFACE-01** When operation surfaces are registered, the system shall include read operations, mutation operations, and meta operations in one registry.

**AGM-SURFACE-02** When logical list-sessions intent is compared with the compiled MCP contract, the system shall account for status, harness, limit, and offset request fields.

**AGM-SURFACE-03** When logical operation definitions declare enum inputs, the compiled-contract audit shall account for every enum addition, omission, and value change.

**AGM-SURFACE-04** When a compiled MCP handler requires a shared operation context, the system shall create that context, defer cleanup, map operation errors to MCP errors, and return successful operation results; pure `list_ops` introspection shall remain context-free.

**AGM-SURFACE-05** When operation discovery is exposed, the system shall publish `list_ops` as an MCP-only meta operation.

**AGM-SURFACE-06** When active harness filters are exposed, the system shall include Claude Code, Codex CLI, AGY, OpenCode, Pi, deprecated Gemini compatibility, and all-harness filtering.

**AGM-SURFACE-07** When logical operation definitions change, the system shall reconcile the compiled MCP wire through exact compatibility records rather than unaccounted hand-maintained divergence.

**AGM-SURFACE-08** While the Cobra tree owns installed plugin command contracts, the operation registry shall not declare a second Skill surface for those commands.

**AGM-SURFACE-09** When the compiled kill-session MCP handler is registered, its request schema shall expose both the recent-activity `force` bypass and the active-harness `confirmed_stuck` confirmation used by the shared operation.

**AGM-SURFACE-10** When provider-visible MCP tools are registered, the system shall route production and contract tests through one private registration seam containing the exact compiled tool set; SDK discovery order shall not be treated as a wire contract.

**AGM-SURFACE-11** When registry and compiled MCP schemas are compared, the system shall preserve exact wire numeric values, reconcile client-visible pagination cursors, and reject repeated non-terminal cursors across discovery, semantic property paths, scalar number versus integer types, complete property constraints and nested array-item schemas, requiredness, enum keyword presence and lossless member values, descriptions, and closed-object behavior while ignoring serialization order and set-like keyword order.

**AGM-SURFACE-12** When a compiled MCP contract differs from registry intent, the system shall require one dimension-specific compatibility record with an exact operation, tool, old value, and new value; matching shall treat schema metacharacters as literal data and reject every unconsumed, blanket, or multiply consumed record.

**AGM-SURFACE-13** When compiled contract differences are audited, the system shall consume every observed difference and every declared compatibility record exactly once so that new drift and stale exceptions fail locally.

**AGM-SURFACE-14** When compiled MCP input is mapped to shared operations, the system shall use production-called adapters that preserve list, search, get, archive, and kill request values, list field masks, and mutation dry-run state.

**AGM-SURFACE-15** When MCP discovery output is audited, the system shall reconcile every compiled logical tool and shall reject every uncompiled advertisement.

**AGM-SURFACE-17** When list, search, get, archive, or kill handlers invoke a shared operation, the system shall propagate the MCP request context separately from input-to-request adaptation.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_control_surface_guardrails.feature`
- Feature: `agm/test/bdd/features/mcp_parity.feature`
- Test consequence: deterministic schema and unit tests own the per-operation
  contract rows above — `agm/internal/surface/codegen_parity_test.go` audits
  compiled tool/schema parity, `agm/internal/surface/ops_test.go` audits the
  registry, and `agm/cmd/agm-mcp-server/surface_contract_compatibility_test.go`,
  `agm/cmd/agm-mcp-server/surface_contract_test.go`, and
  `agm/cmd/agm-mcp-server/tools_test.go` audit production SDK registration,
  request mapping, request-context cancellation, and discovery output. The two
  features above cover co-located SPEC coverage and operation discovery parity.
