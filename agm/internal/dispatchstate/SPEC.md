# Dispatch State Specification

<!-- Last audited at: 2026-08-17 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/dispatchstate` — where AGM records which Dispatch session receives worker completions, and how it reads the provider quota snapshot that routing decisions consult

## Overview

`agm/internal/dispatchstate` owns two pieces of host-local state that outlive
any single AGM process:

- **The completion relay target** — the Dispatch session that finished worker
  output should be routed to. Without a durable target, completions were
  written against whichever session happened to be hardcoded, so a restarted
  or re-created Dispatch never saw them.
- **The quota snapshot** — the provider quota state written by the CodexBar
  meter, read here so routing and stall handling can tell a throttled
  provider from a healthy one without re-querying it.

Both are deliberately plain files under the user's home directory rather than
rows in the AGM Dolt store: they must stay readable when the session store is
unreachable, which is exactly when completion routing matters most.

Consumers: `agm/cmd/agm/completion_surface.go`, `agm/cmd/agm/watch_stalled.go`,
`agm/internal/ops` completion and stall-recovery paths, and the
`agm_get_completion_relay_target` / `agm_set_completion_relay_target` /
`agm_get_quota_status` MCP tools.

## EARS Requirements

### Relay target resolution

- WHEN `AGM_COMPLETION_RELAY_TARGET` holds a non-blank value, the system SHALL resolve that value as the relay target and report the source as `env:AGM_COMPLETION_RELAY_TARGET`.
- WHEN the environment variable is unset or blank and `~/.agm/completion-relay-target` holds non-blank contents, the system SHALL resolve the file contents as the relay target and report the source as `file`.
- WHEN neither the environment variable nor the state file yields a non-blank value, the system SHALL resolve the caller-supplied fallback and report the source as `fallback`.
- WHERE a resolved target carries surrounding whitespace, the system SHALL trim it so a trailing newline never becomes part of the session identifier.
- WHERE the state file exists but cannot be read, the system SHALL fall through to the fallback rather than failing resolution, because a completion routed to the fallback is recoverable while a hard error loses it.

### Relay target persistence

- WHEN asked to set a blank target, the system SHALL reject the write and return an error so an empty write cannot silently disable relaying.
- WHEN persisting a target, the system SHALL create the containing directory with mode `0700` and write the state file with mode `0600`.
- WHEN persisting a target, the system SHALL write a trailing newline and report the resulting path so the file stays diffable by hand.

### Quota snapshot

- WHEN the quota state file is absent, the system SHALL report the snapshot unavailable with reason `quota state not found` rather than returning an error.
- WHEN the quota state file cannot be parsed, the system SHALL report the snapshot unavailable with a reason naming the parse failure.
- WHERE a provider is named, the system SHALL select that provider's entry by case-insensitive match on `provider`, `source_id`, `sourceId`, or `family`, or from a top-level key of the same name.
- WHERE a named provider matches no entry, the system SHALL fall back to the whole payload.
- WHEN the snapshot carries a parseable RFC 3339 timestamp older than 30 minutes, the system SHALL mark the result stale.
- WHEN the snapshot is stale, reports `throttled`, `warning`, or `overspending`, carries a `throttled` breaker state, or reports low remaining quota, the system SHALL set the warning flag.

### Invariants

- The system SHALL NOT mutate either piece of state on a read path.
- The system SHALL NOT return an error from either read path for absent state, because both are consulted on completion and stall paths where a hard failure would drop the output being routed.
- The system SHALL prefer the environment variable over the state file so an operator can override relay routing for a single process without editing host state.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature` (changed-package SPEC coverage gate)

## Non-goals

- Validating that a relay target names a session that currently exists. The
  target is resolved on the completion path, where the session's liveness is
  checked by the caller against the session store.
- Writing the quota snapshot. This package only reads it; the CodexBar meter
  owns its production and format.
