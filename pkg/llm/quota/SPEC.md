# Provider Quota Meter Specification

<!-- Last audited at: 2026-08-11 -->

## Overview

`pkg/llm/quota` reads per-provider remaining quota from a local meter and
reduces it to a routing verdict. `CodexBarReader` parses the
`codexbar dashboard --identity redacted` snapshot, `Evaluate` turns a reading
into a per-family decision under operator thresholds, and `Meter` caches the
reading off the request path and exposes it as a candidate ordering.

Every unknown is routed as though the package were absent: a missing,
unreadable, stale, or unauthenticated provider never loses traffic and never
loses a candidate slot. See `docs/adr/ADR-038-codexbar-quota-routing.md`.

## EARS Requirements

**LLM-QUOTA-01** When a quota reading is requested from the CodexBar reader, the system shall invoke the configured command with the dashboard subcommand and a redacted identity argument.

**LLM-QUOTA-02** When the meter command fails, the system shall return an error that omits the command's standard error output.

**LLM-QUOTA-03** When a dashboard payload declares a schema version other than the supported version, the system shall reject the payload.

**LLM-QUOTA-04** When a dashboard payload is parsed, the system shall record the source name, source version, generation time, and the source's own staleness hint.

**LLM-QUOTA-05** When a dashboard provider is parsed, the system shall map its source identifier to a provider family through the configured alias table and shall fall back to the source identifier when no alias matches.

**LLM-QUOTA-06** When a dashboard provider reports at least one usage window, the system shall classify it as readable regardless of any accompanying error.

**LLM-QUOTA-07** When a dashboard provider reports no usage window, the system shall classify it as disabled, authentication-required, or unavailable, and shall never classify it as exhausted.

**LLM-QUOTA-08** When more than one source identifier maps to one provider family, the system shall prefer a readable reading over an unreadable one and shall prefer the most constrained reading when both are readable.

**LLM-QUOTA-09** When a timestamp in the payload cannot be parsed, the system shall retain the remaining usable reading rather than failing the parse.

**LLM-QUOTA-10** When a provider family is evaluated, the system shall derive the remaining percentage from the most constrained usage window and shall name that window in the decision.

**LLM-QUOTA-11** When the remaining percentage is at or below the avoid threshold, the system shall classify the family as avoid.

**LLM-QUOTA-12** When the remaining percentage is at or below the deprioritize threshold and above the avoid threshold, the system shall classify the family as deprioritized.

**LLM-QUOTA-13** When no reading, no matching family, no usable reading, or a reading older than the maximum age is evaluated, the system shall classify the family as unknown and shall state in the decision reason that the cause is not exhaustion.

**LLM-QUOTA-14** When a decision is scored into a routing band, the system shall band it by quartile of remaining quota, shall score an unknown decision into the first band, and shall floor the band when an explicit threshold classified the family.

**LLM-QUOTA-15** When candidate models are ordered, the system shall sort them by routing band, shall preserve the configured order within a band, and shall return every candidate it was given.

**LLM-QUOTA-16** When the cached reading is requested, the system shall return it without blocking on the underlying source and shall trigger a background refresh once the reading is older than the refresh interval.

**LLM-QUOTA-17** When a refresh fails, the system shall retain the previous reading and shall report the failure to the caller.

**LLM-QUOTA-18** When the meter has no reader, or the meter reference is nil, the system shall report capacity for every model and shall return candidates in their configured order.

**LLM-QUOTA-19** When a model identifier cannot be mapped to a provider family, the system shall classify it as unknown and shall report capacity for it.

**LLM-QUOTA-20** When capacity is checked for a model, the system shall report an absence of capacity only for a family classified as avoid.

**LLM-QUOTA-21** When burn-rate readings are collected, the system shall request them without a full account identity and shall retain no account identifier from that response.

**LLM-QUOTA-22** When a burn-rate read fails, the system shall retain the quota windows already parsed.

**LLM-QUOTA-23** When a provider reports burn rate for more than one window, the system shall retain the reading that will exhaust first.

**LLM-QUOTA-24** When a provider's remaining quota is at or below the halt floor, the system shall open the guardrail for that provider.

**LLM-QUOTA-25** When a provider is overspending and its remaining quota is at or below the spike floor, the system shall open the guardrail and record that burn rate drove the verdict.

**LLM-QUOTA-26** When a provider is overspending with remaining quota above the spike floor, or its remaining quota is at or below the throttle floor, the system shall throttle rather than open the guardrail.

**LLM-QUOTA-27** When no usable reading exists for a provider, the system shall leave the guardrail closed.

**LLM-QUOTA-28** When work is admitted against a throttled provider, the system shall refuse admissions beyond the configured hourly allowance and shall report when to retry.

**LLM-QUOTA-29** When the guardrail position is evaluated without admitting work, the system shall not consume the hourly allowance.

**LLM-QUOTA-30** When a reading is published, the system shall write it atomically and shall record the schema version, generation time, per-provider verdicts, and every sub-budget.

**LLM-QUOTA-31** When a published reading is absent, the system shall report that condition distinctly from a corrupt or unrecognised one.

**LLM-QUOTA-32** When a published reading declares an unsupported schema version, the system shall reject it.

**LLM-QUOTA-33** When a spawn is gated, the system shall refuse it only for a provider whose published guardrail is open in a reading within the gating age.

**LLM-QUOTA-34** When the published reading is absent, stale, corrupt, unreadable for the provider, or the model maps to no provider family, the system shall allow the spawn.

**LLM-QUOTA-35** When the guardrail override environment variable is set to a disabling value, the system shall allow the spawn without consulting any reading.

**LLM-QUOTA-36** When a spawn is refused, the system shall report the constraining provider, the reason, the window reset time when known, and the override.

## BDD Traceability

- Feature: `agm/test/bdd/features/llm_runtime_guardrails.feature`
- Package tests: `pkg/llm/quota/codexbar_test.go`
- Package tests: `pkg/llm/quota/policy_test.go`
- Package tests: `pkg/llm/quota/meter_test.go`
- Package tests: `pkg/llm/quota/breaker_test.go`
- Package tests: `pkg/llm/quota/spawngate_test.go`
- Router integration tests: `pkg/llm/router/quota_test.go`
- Captured live payload: `pkg/llm/quota/testdata/codexbar-dashboard-live.json`
