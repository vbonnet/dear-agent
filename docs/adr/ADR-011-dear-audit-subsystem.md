# ADR-011: Scheduled repository audit subsystem

Status: Accepted (2026-06-16; verified 2026-07-17; amended 2026-08-28)

## Context

CI answers whether one revision passes its gates. It does not own recurring
repository checks, finding deduplication, remediation suggestions, or trend
queries.
Embedding schedules inside each check would also couple policy to one runtime.

## Decision

`pkg/audit` owns addressable checks, structured findings, and SQLite-backed run
history. Checks declare stable IDs and recommended cadence but own no clock.
Operators schedule `workflow-audit run` through cron, CI, or workflow wrappers.
Finding and remediation-suggestion generation remain separate stages, and finding
fingerprints prevent the same unresolved problem from inflating counts across
runs. Remediation fields are suggestion-only data: the audit runner does not
execute commands, open pull requests or issues, or alter finding state on their
behalf. Strategies are closed handling hints, not evidence that automation, a
command, a patch, a pull request, or an issue is applicable or authorized.
Command, patch, title, and body are optional operator context; a patchless PR
suggestion can recommend investigation or PR-producing work. Typed `Store`
writes enforce the closed strategy vocabulary. Direct SQL is an out-of-band
corruption path; finding reads, re-emission, and lifecycle transitions reject
an unknown stored value before exposing or mutating it.

A side-effecting dispatcher would need its own charter and a proven live producer
and consumer. It must define durable intent and outcome records, idempotency,
leases or equivalent ownership, retries, crash recovery, reconciliation, and
operator-visible evidence before it can consume audit suggestions. Those
responsibilities do not belong to `pkg/audit`.

The v1 storage schema is additive and owns exactly `audit_findings`,
`audit_runs`, and `audit_proposals`. It does not modify workflow tables,
columns, or indexes. Operators may apply those tables to `runs.db`, but
`workflow-audit` defaults to `.dear-agent/audit.db` and does not require
co-location with workflow state. Later schema changes require explicit
migrations; `pkg/audit/schema.sql` remains the canonical v1 snapshot.

This is repository audit, not the Process DEAR lifecycle defined by
[ADR-035](ADR-035-dear-terminology-disambiguation.md).

## Alternatives

Independent scripts duplicate lifecycle and storage. A built-in scheduler would
make the package responsible for host policy and uptime. Raw log lines cannot
support deduplication or reopening.

## Consequences

Operators must supply scheduling. Shared records make audit results queryable
and allow normal workflow steps to consume them. `pkg/audit` and its command
tests own verification. The unreleased inline remediator interface and the
corresponding `workflow-audit run --dry-run` flag were removed because production
only installed a no-op. Source callers that experimented with that interface must
move side effects into an independently durable workflow and treat stored
remediation fields as input suggestions.
