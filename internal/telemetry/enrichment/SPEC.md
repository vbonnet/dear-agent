# Telemetry Enrichment Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/telemetry/enrichment` decorates telemetry events with plugin,
ecphory, and sanity-check context. Enrichers are isolated by timeout wrappers
and circuit breakers so telemetry analysis can improve observability without
making the primary agent workflow fragile.

## Requirements

**TEL-ENRICH-01** When an enricher receives an event type it does not own, the system shall return the original event unchanged.

**TEL-ENRICH-02** When plugin execution events are enriched, the system shall add a salted prompt hash, expected plugin names, loaded plugin names, and missing plugin names when expected plugins were not loaded.

**TEL-ENRICH-03** When expected plugins are detected from prompts, the system shall only report plugins that are available in the current enrichment context.

**TEL-ENRICH-04** When ecphory retrieval events are enriched with an ecphory result, the system shall add prompt hash, retrieved engram count, filtered candidate count, token budget use, retrieval strategy, and token utilization percentage.

**TEL-ENRICH-05** When ecphory retrieval events lack an ecphory result, the system shall return the original event unchanged.

**TEL-ENRICH-06** When session-end events are enriched, the system shall emit separate sanity-check events for plugin loading, version compatibility, and ecphory coverage when applicable.

**TEL-ENRICH-07** When sanity-check event emission fails, the system shall log the failure and keep the original session-end event successful.

**TEL-ENRICH-08** When an enricher panics or exceeds its timeout, the system shall return the original event with an error instead of propagating the panic or blocking indefinitely.

**TEL-ENRICH-09** When an enricher repeatedly fails, the system shall open its circuit breaker, fail fast while open, probe after the configured timeout, and close only after the configured half-open success threshold.

**TEL-ENRICH-10** When an enrichment pipeline runs multiple enrichers, the system shall feed each successful enriched event to the next enricher and continue with the current event after timeout, panic, circuit-open, or other enrichment errors.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
