# Design Spike: Agent-Generated OTel Grafana Dashboard

- **Bead:** ce-cv8o (depends on ce-cjc0, OTel activation)
- **Status:** Proposed (design spike — no implementation)
- **Date:** 2026-06-20
- **Decision driver:** Make the VROOM mesh observable. Agents that instrument
  code know what matters; they should also author the dashboard view.

## Context

dear-agent emits OpenTelemetry **traces** via `pkg/otelsetup` (`InitTracer`) and
**metrics** via `internal/telemetry` (`InitMeter`). Both use a single **OTLP
gRPC exporter** driven by `OTEL_EXPORTER_OTLP_ENDPOINT` (`:4317`). When the env
var is unset, both install no-op providers — telemetry is opt-in everywhere.
There is **no Prometheus exporter and no `/metrics` scrape endpoint** anywhere in
the tree; the repo standardizes on push-only OTLP gRPC. Local dev points at a
Jaeger all-in-one (`cmd/otel-local`) for traces.

## 1. Metric inventory (what exists today)

Real OTel metric instruments emitted (counters/histograms):

| Metric | Type | Unit | Key attributes |
|---|---|---|---|
| `agent.tasks.active` | UpDownCounter | `{task}` | provider, model |
| `agent.tasks.completed` | Counter | `{task}` | provider, model, status |
| `agent.tokens.used` | Counter | `{token}` | provider, model |
| `agent.stall.duration_ms` | Histogram | ms | — |
| `agent.eval.score` | Histogram | `1` | gen_ai.eval.name |
| `agent.eval.cases_generated` | Counter | `{case}` | — |
| `dear.cycle.duration_ms` | Histogram | ms | dear.phase |
| `mergeloop.time_to_merge` | Histogram | ms | — |
| `mergeloop.pr.stall` | Counter | `{pr}` | state |
| `mergeloop.human_escalation` | Counter | — | reason |
| `mergeloop.agent_spawned` | Counter | `{session}` | kind |
| `mergeloop.merged` | Counter | `{pr}` | — |
| `mergeloop.tick.open_prs` | Counter* | `{pr}` | — |
| `vroom.escalation.events` | Counter | `{event}` | phase, kind, disposition |
| `vroom.escalation.raised` | Counter | `{escalation}` | kind, topic |
| `vroom.escalation.dispatched_to_human` | Counter | `{escalation}` | — |
| `vroom.escalation.latency_ms` | Histogram | ms | — |

`*` `mergeloop.tick.open_prs` is declared as a Counter but used as a per-tick
gauge sample — it should become an ObservableGauge (see "missing").

**Traces only (no metrics):** `vroom.session.spawn/create/boot/loop/recreate`,
`vroom.ensure_sessions`, `agm.session.register/kill/state_set/list`,
`safemerge.*`, `safepr.*`, `wayfinder.phase.*`.

**Missing (the bead's target signals).** The five VROOM-health signals the bead
calls out — **swap, FD pressure, heartbeat freshness, tick latency, dispatch
lag** — exist today only as *span attributes* (`freshness_seconds`,
`fd_before/after/fraction`), not as metric instruments. They are invisible to a
metrics dashboard. W0 should add gauges/histograms for them. This is the
single largest gap.

## 2. Proposed dashboard panels

| # | Panel | Visualization | Query (PromQL-style) | Threshold |
|---|---|---|---|---|
| 1 | Heartbeat age | Stat | `max(heartbeat_age_seconds)` *(new)* | red > 300s |
| 2 | Active agent tasks | Time series | `sum(agent_tasks_active) by (model)` | — |
| 3 | Token burn rate | Time series | `rate(agent_tokens_used_total[5m])` by model | — |
| 4 | PR merge throughput | Stat / bar | `sum(rate(mergeloop_merged_total[1h]))` | — |
| 5 | Time-to-merge p50/p95 | Heatmap | `histogram_quantile(0.95, mergeloop_time_to_merge_bucket)` | — |
| 6 | Worker spawn rate | Time series | `rate(mergeloop_agent_spawned_total[15m])` by kind | — |
| 7 | Escalations to human | Stat | `increase(vroom_escalation_dispatched_to_human_total[1h])` | red > 0 |
| 8 | Escalation latency p95 | Time series | `histogram_quantile(0.95, vroom_escalation_latency_ms_bucket)` | — |
| 9 | Open PRs | Stat | `max(mergeloop_tick_open_prs)` *(→ gauge)* | amber > 10 |
| 10 | FD fraction | Gauge | `max(fd_fraction)` *(new)* | red > 0.8 |

Panels 2–9 are queryable **today**; 1 and 10 depend on W0 adding the missing
instruments. The OTLP→Prometheus naming convention applies (`_total` suffix on
monotonic counters, `_bucket` on histograms, dots→underscores).

## 3. Data-source assumption

Given the **push-only OTLP gRPC** posture, the simplest path is *not* Prometheus
scraping (there is no endpoint to scrape). Instead:

```
binaries --OTLP/gRPC:4317--> OTel Collector / Grafana Alloy --remote_write--> Mimir/Prometheus --> Grafana (PromQL)
```

Alloy receives OTLP, converts delta→cumulative, and remote-writes to a
Prometheus-compatible TSDB (self-hosted Mimir or **Grafana Cloud**, which exposes
a native OTLP endpoint and removes the Alloy hop). **Recommendation:** Grafana
Cloud OTLP for the spike — point `OTEL_EXPORTER_OTLP_ENDPOINT` at the Cloud
gateway, zero extra infra. Traces ride the same pipe to Tempo.

## 4. JSON generation approach

| Option | Pros | Cons |
|---|---|---|
| Hand-written JSON | Simple, no deps | Brittle, verbose, drifts |
| **Go generator** (existing pattern) | Reuses repo's metric constants as source of truth; agent emits panels next to instruments; testable | Must model Grafana schema |
| Grafonnet (Jsonnet) | Idiomatic, composable | New toolchain/language in a Go repo |

**Recommendation:** a small **Go generator** under `cmd/`. It already fits the
repo — instrument names live in Go constants, so a generator can derive panel
queries from the same source and a `go test` golden-file check prevents drift.
This realizes the bead's premise: the agent that adds an instrument adds its
panel in the same change.

## 5. Alert rules

Page the operator on:

- **Heartbeat silent > 600s** — `max(heartbeat_age_seconds) > 600` (mesh stalled).
- **Escalation backlog** — `increase(vroom_escalation_dispatched_to_human_total[10m]) > 0`.
- **CI failure rate > 50%** — derived from mergeloop stall/merge ratio.
- **FD exhaustion** — `max(fd_fraction) > 0.9` (file-descriptor leak).
- **Ghost-text / spawn storm** — `rate(mergeloop_agent_spawned_total[1h]) > 1`.

Alerts are defined as Grafana unified-alerting rules co-located with the
dashboard provisioning.

## 6. W0 (implementation bead) requirements

1. **New instruments** for the missing signals: `heartbeat_age_seconds` (gauge),
   `fd_fraction` (gauge), swap usage, dispatch lag (histogram), and promote
   `mergeloop.tick.open_prs` to an ObservableGauge.
2. **Dashboard JSON** committed to the repo (`observability/dashboards/`).
3. **Go generator** + golden-file test.
4. **Provisioning config** (`observability/provisioning/`): datasource YAML +
   dashboard provider pointing at the JSON.
5. **Grafana Cloud (or Alloy) datasource** setup documented in a runbook.
6. **Alert rules** YAML for the five conditions above.

## Decision

Adopt a Go dashboard generator over Grafana Cloud OTLP, deferring the missing
instrument work to W0. No code in this spike.
