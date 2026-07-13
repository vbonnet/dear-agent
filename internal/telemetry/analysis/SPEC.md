# Telemetry Analysis Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/telemetry/analysis` parses telemetry JSONL streams into typed events
for retrospective analysis. It favors partial progress: malformed records are
reported without preventing valid neighboring events from being consumed.

## Requirements

**TEL-ANALYSIS-01** When a JSONL telemetry file is parsed asynchronously, the system shall accept a context, return event and error channels, and close both after scanning completes or the context is canceled.

**TEL-ANALYSIS-02** When an input line is empty, the system shall skip it without emitting an event or parse error.

**TEL-ANALYSIS-03** When an input line contains malformed JSON, the system shall emit a line-numbered parse error and continue scanning subsequent lines.

**TEL-ANALYSIS-04** When a parsed event omits a schema version, the system shall default the event schema version to `1.0.0`.

**TEL-ANALYSIS-05** When scanner or parser errors exceed the error channel capacity, the system shall avoid blocking event parsing.

**TEL-ANALYSIS-06** When synchronous JSONL parsing is requested, the system shall return all valid parsed events and aggregate any parse errors into the returned error value.

**TEL-ANALYSIS-07** When parser goroutines panic unexpectedly, the system shall recover and report the panic through the error channel.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
