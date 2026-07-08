# Cost Tracking Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/costtrack` is the shared cost and budget enforcement package for LLM API
usage. It owns model pricing, cost calculation, cost sinks, budget config,
period accounting, persisted budget state, per-session token caps, and
threshold alerts.

## EARS Requirements

**COT-01** When pricing is requested for a model alias, the system shall resolve the alias before looking up the pricing table.

**COT-02** When pricing is requested for an unknown model, the system shall return zero-cost pricing rather than guessing.

**COT-03** When token costs are calculated, the system shall multiply input, output, cache-write, and cache-read tokens by their respective per-token rates and sum them into total cost.

**COT-04** When cache metrics are calculated with cache tokens, the system shall report cache hit rate and estimated savings from cache reads and writes.

**COT-05** When a file cost sink records an event, the system shall write one JSON object per line with timestamp, operation, provider, model, tokens, cost, and optional component, cache, context, and request ID fields.

**COT-06** When a file cost sink closes, the system shall sync and close the file and shall not close it twice.

**COT-07** When budget config is loaded, the system shall parse YAML and default missing top-level limits to an empty limit set.

**COT-08** When effective budget limits are requested, the system shall apply model limits over project limits over defaults, using only positive override values.

**COT-09** When budget periods are calculated, the system shall derive daily, Monday-based weekly, and monthly bounds in the caller's time zone.

**COT-10** When budget logs are checked, the system shall skip malformed lines, out-of-window entries, non-matching models, and non-matching project contexts.

**COT-11** When `ENGRAM_BUDGET_OVERRIDE` is set to `true`, the system shall allow budget checks without reading spend.

**COT-12** When daily budget is calculated from a weekly limit, the system shall smooth the weekly budget across elapsed days and clamp available spend to zero when prior spend exceeds allocation.

**COT-13** When budget state is saved, the system shall create the state directory with private permissions and atomically replace the JSON state file.

**COT-14** When budget state is loaded for an expired week or changed weekly limit, the system shall reset weekly spend and session token state.

**COT-15** When a pre-check sees exhausted daily budget, over-budget estimated cost, or an exhausted per-session token cap, the system shall deny the request with a reason.

**COT-16** When usage is recorded, the system shall persist spend and session tokens in one state update and fire daily threshold alerts at 50, 75, 90, and 100 percent.

**COT-17** When threshold alerts have already fired for the same project, model, period, and threshold, the system shall suppress duplicate alerts until reset.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_monitoring_guardrails.feature`
- Package tests: `pkg/costtrack/*_test.go`

