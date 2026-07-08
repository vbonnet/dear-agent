# Agent Telemetry Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/telemetry/agent` records agent launch and completion telemetry for
analysis of prompt specificity, outcomes, token use, retries, and task context.
It supports SQLite persistence and optional EventBus publication so callers can
use durable telemetry, live event fanout, or both.

## Requirements

**AGENT-TEL-01** When agent launch telemetry is recorded with storage configured, the system shall persist prompt metadata, model, task description, session IDs, parent agent IDs, extracted prompt features, and a timestamp.

**AGENT-TEL-02** When agent launch telemetry is recorded with an EventBus configured, the system shall publish an `agent_launch` event containing launch identity, prompt metadata, extracted feature values, model, task, session context, and timestamp.

**AGENT-TEL-03** When EventBus publication fails, the system shall log a warning and still return the persisted launch ID.

**AGENT-TEL-04** When storage is not configured, the system shall still publish EventBus launch events when an EventBus is available.

**AGENT-TEL-05** When launch storage is not configured and callers query launches or statistics, the system shall return a storage-not-configured error.

**AGENT-TEL-06** When completion telemetry is recorded with storage configured, the system shall update outcome, output token count, retry count, error text, and duration for the existing launch ID.

**AGENT-TEL-07** When completion telemetry is recorded for an unknown launch ID, the system shall report that the launch was not found.

**AGENT-TEL-08** When launch records are queried, the system shall support outcome, model, since-time, and limit filters, defaulting to 100 records and ordering newest first.

**AGENT-TEL-09** When prompt features are extracted, the system shall derive word count, token count, specificity score, example presence, constraint presence, and context embedding score without storing additional prompt-derived raw text.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
