# Dispatch State Specification

<!-- Last audited at: 2026-08-17 -->

**Version:** 1.1
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

- WHEN `~/.agm/completion-relay-target` holds non-blank contents, the system SHALL resolve those contents as the relay target and report the source as `file`.
- WHEN the state file yields no non-blank value and `AGM_COMPLETION_RELAY_TARGET` holds a non-blank value, the system SHALL resolve that value and report the source as `env:AGM_COMPLETION_RELAY_TARGET`.
- WHEN neither the state file nor the environment variable yields a non-blank value, the system SHALL resolve the caller-supplied fallback and report the source as `fallback`.
- WHERE a resolved target carries surrounding whitespace, the system SHALL trim it so a trailing newline never becomes part of the session identifier.
- WHERE the state file exists but cannot be read, the system SHALL fall through to the next source rather than failing resolution, and SHALL report the failure as a reason so a degraded resolve is distinguishable from no override being set.
- WHERE the state file exists but is empty, the system SHALL fall through to the next source and report the empty state as a reason, because a blank target cannot have been set deliberately.

### Relay target persistence

- WHEN asked to set a blank target, the system SHALL reject the write and return an error so an empty write cannot silently disable relaying.
- WHEN persisting a target, the system SHALL write the state file atomically, so a concurrent reader observes either the previous target or the complete new one and never an empty or partial value.
- WHEN persisting a target, the system SHALL enforce directory mode `0700` and file mode `0600`, including when the directory or file already exists with broader modes.
- WHEN persisting a target, the system SHALL write a trailing newline and report the resulting path so the file stays diffable by hand.
- WHERE a target is persisted, the system SHALL make it effective for a running watcher without a restart, in every configuration including one where `AGM_COMPLETION_RELAY_TARGET` is exported.

### Quota snapshot

- WHEN the quota state file is absent, the system SHALL report the snapshot unavailable with reason `quota state not found` rather than returning an error.
- WHEN the quota state file cannot be parsed, the system SHALL report the snapshot unavailable with a reason naming the parse failure.
- WHERE a provider is named, the system SHALL select that provider's entry by case-insensitive match on a top-level key of the same name, or on `provider`, `source_id`, `sourceId`, or `family` within a `providers` collection.
- WHERE the snapshot carries a structured `providers` collection and the named provider matches no entry, the system SHALL report the snapshot unavailable with a reason naming the missing provider, and SHALL NOT fall back to the whole payload.
- WHERE the snapshot carries no provider structure, the system SHALL treat the payload as that provider's data.
- WHEN the snapshot carries a parseable RFC 3339 timestamp older than 30 minutes, the system SHALL mark the result stale.
- WHEN the snapshot carries no capture time, or a capture time that is not RFC 3339, the system SHALL mark the result stale and give a reason, because pacing work from an unbounded-age snapshot is the expensive direction of a wrong answer.
- WHERE the selected provider entry carries its own capture time, the system SHALL prefer it over a top-level one.
- WHEN the snapshot is stale, reports `throttled`, `warning`, or `overspending`, carries a `throttled` breaker state, or reports low remaining quota, the system SHALL set the warning flag.

### Invariants

- The system SHALL NOT mutate either piece of state on a read path.
- The system SHALL NOT return an error from either read path for absent state, because both are consulted on completion and stall paths where a hard failure would drop the output being routed.
- The system SHALL prefer the state file over the environment variable, so the surface that advertises live retargeting is effective on hosts that export a startup default.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature` (changed-package SPEC coverage gate)

## Non-goals

- Validating that a relay target names a session that currently exists. This
  package must stay usable when the session store is unreachable, so the
  check belongs to the setter surfaces: `agm_set_completion_relay_target`
  rejects an identifier that definitively resolves to no session, and lets
  the write through with a warning when the store cannot be consulted.
- Matching a relay target against a completion event. A target may be a
  session name, a full session ID, or a UUID prefix, and deciding whether an
  event names the target session is the watcher's self-filter
  (`agm/cmd/agm/completion_surface.go`), which must apply one target
  snapshot to both filtering and delivery.
- Writing the quota snapshot. This package only reads it; the CodexBar meter
  owns its production and format.
