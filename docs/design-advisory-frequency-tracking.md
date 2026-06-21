# Design: Durable Advisory Frequency Tracking

**Bead:** ce-3qxi · **Status:** Investigation only (no implementation) · **Date:** 2026-06-20

## Problem

The human operator wants to graph advisory interventions over time and break
them down by category — "process drift", "swap crisis", "DoD violation",
"comms failure". Today advisories are delivered as prose and dissolve into the
trail as untyped `supervisor.tick` notes. They are not addressable: you cannot
ask "how many DoD violations this week?" without re-reading every tick by eye,
and that history is the first thing lost when a supervisor's context compacts.
We need a typed, append-only record that outlives the context window and is
queryable by category and time range.

## 1. Categorization Schema

Each advisory carries one `category` code and a `severity`. The codes unify the
candidates from the bead and the dispatch advisory:

| Code | Description |
|---|---|
| `process_drift` | Agent not following an established pattern (skipped repeat-back, wrong workflow) |
| `dod_violation` | Bead closed without meeting its Definition of Done |
| `swap_crisis` | Swap/FD/memory pressure requiring intervention (was `resource_crisis`) |
| `comms_failure` | Telephone-game drift, misunderstood or mis-relayed instructions |
| `ghost_text_kill` | Worker emitting ghost text / stalled output, killed |
| `ci_failure` | PR blocked on red CI requiring manual remediation |
| `dispatch_skip` | Dispatcher frozen or skipping eligible beads |
| `worker_stall` | Dispatched worker frozen, not making progress |
| `pr_conflict` | Conflicting PRs requiring manual remediation |
| `supervisor_stale` | Orch/overseer/meta-o heartbeat failure |

**Severity:** `P0` (active harm — work lost, mesh stalled, must act now),
`P1` (intervention needed this cycle), `informational` (logged for trend, no
action). Severity is orthogonal to category: a `swap_crisis` can be P0 or
merely informational depending on margin.

The code set is closed and versioned (`schema_version: advisory.v1`); adding a
category is a schema bump so dashboards never silently drop unknown codes.

## 2. Storage Mechanism

Options considered:

- **Untyped `supervisor.tick` note** (status quo) — durable but unparseable;
  the category is buried in prose. Rejected as the source of truth.
- **OTel counter by category** — great for live Grafana panels, but metrics are
  lossy (no per-event note, sampling/retention windows) and need a collector
  running. Wrong as the durable record.
- **Dedicated `advisory.jsonl`** — clean isolation, but a second append target
  the supervisors must learn and the operator must correlate against the trail.
- **Structured trail record** — a new `kind: supervisor.advisory` line in the
  existing `trail.jsonl`.

**Recommendation: a structured `supervisor.advisory` record appended to
`trail.jsonl`.** The trail is already the append-only, compaction-surviving
spine every supervisor writes to; reusing it means zero new plumbing, one
correlation timeline, and the audit ordering comes for free. OTel counters are
retained as an *optional mirror* for live dashboards (emit on append), but the
jsonl line is canonical. This matches the storage split the spike corpus
already assumes — append-only file = durable truth, metrics = live view.

## 3. Record Schema

```json
{
  "schema_version": "advisory.v1",
  "ts": "2026-06-20T18:32:11Z",
  "kind": "supervisor.advisory",
  "category": "dod_violation",
  "severity": "P1",
  "session": "meta-orchestrator",
  "target_bead": "ce-4h06",
  "note": "PR #592 merged but acceptance criterion 3 unverified; reopened for check.",
  "resolved_at": null
}
```

`category` and `severity` are the load-bearing fields — they are what the
operator groups and filters on. `ts` is RFC3339 UTC (sortable, the trail
convention). `session` attributes the advisory to the supervisor that raised it
(meta-orchestrator / orchestrator / overseer). `target_bead` is optional and
links the advisory to the bead it concerns. `note` is the human-readable
one-liner. `resolved_at` starts `null` and is set by a follow-up
`supervisor.advisory` carrying the same `target_bead` + category with a
`resolved_at` timestamp, giving mean-time-to-resolution as a derived metric
without mutating the immutable original line.

## 4. Query Interface

Because the record is one jsonl line per advisory, the operator graphs it with a
`jq` pipeline — no service required:

```sh
# advisories per ISO week by category
jq -r 'select(.kind=="supervisor.advisory")
  | [(.ts[0:10]), .category] | @tsv' ~/.agm/vroom/trail.jsonl \
  | sort | uniq -c

# P0/P1 only, this week, with the note
jq -r 'select(.kind=="supervisor.advisory" and .severity!="informational")
  | "\(.ts)  \(.severity)  \(.category)  \(.note)"' trail.jsonl
```

A thin `tools/advisory-report` wrapper can format the same pipeline into a
weekly table or CSV for the operator's spreadsheet/graph. The OTel mirror (§2)
feeds a Grafana panel for those who want it live.

## 5. Compaction Survival

The mechanism survives context compaction because the record never lives in the
context window: it is appended to `trail.jsonl` on disk the moment the advisory
is raised. When a supervisor's context compacts, the file is untouched — the
full history is recovered by reading the file, not the conversation. Two
reinforcements: (a) the meta-orchestrator's periodic roll-up tick can emit an
`informational` advisory summarizing counts since the last roll-up, so even a
truncated trail tail retains aggregates; (b) an optional weekly summary bead
snapshots the counts so the operator has a checkpoint independent of jsonl
retention.

## 6. W0 Requirements (Implementation Bead)

1. `schemas/advisory.v1.json` — versioned schema for the record above; closed
   `category` enum, `severity` enum, mandatory `schema_version`.
2. An append helper all three supervisors call to write a validated
   `supervisor.advisory` line (validate-then-append; reject unknown category).
3. `resolved_at` follow-up convention + a derive helper for MTTR.
4. Optional OTel counter mirror (`advisory_total{category,severity}`) emitted on
   append.
5. `tools/advisory-report` — jq-backed weekly breakdown by category, CSV/table
   output for graphing.
6. Acceptance: an advisory with an unknown category fails validation; a written
   advisory is queryable by category and ISO week from `trail.jsonl`; the
   operator can produce an "advisories per week by category" graph.

## Companion Beads

[[ce-228u]] · [[ce-ynyb]] spike pattern adoption · [[ce-g8h2]] repeat-back
protocol (a source of `comms_failure` advisories).
